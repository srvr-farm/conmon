package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/mcallan/conmon/internal/sysmon/collector"
	"github.com/mcallan/conmon/internal/sysmon/config"
	"github.com/mcallan/conmon/internal/sysmon/identity"
	"github.com/mcallan/conmon/internal/sysmon/metrics"
	"github.com/mcallan/conmon/internal/sysmon/push"
	"github.com/mcallan/conmon/internal/sysmon/systemd"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

type hostCollector interface {
	Snapshot(ctx context.Context) (collector.HostSample, error)
}

type statusReader interface {
	Status(ctx context.Context, unit string) (systemd.UnitStatus, error)
}

type cgroupUsageReader interface {
	ReadUsage(controlGroup string) (systemd.Usage, error)
}

type pusher interface {
	Push(ctx context.Context, host string, reg prometheus.Gatherer) error
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type timeTicker struct {
	t *time.Ticker
}

func (t timeTicker) C() <-chan time.Time { return t.t.C }
func (t timeTicker) Stop()               { t.t.Stop() }

type monotonicNowFunc func() (uint64, error)

func defaultMonotonicNowUS() (uint64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0, err
	}
	return uint64(ts.Sec)*1_000_000 + uint64(ts.Nsec)/1_000, nil
}

type systemctlShowRunner interface {
	Show(ctx context.Context, unit string) ([]byte, error)
}

type systemctlRunner struct{}

func (systemctlRunner) Show(ctx context.Context, unit string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", unit,
		"--no-page",
		"--property=Id",
		"--property=ActiveState",
		"--property=UnitFileState",
		"--property=ControlGroup",
		"--property=ActiveEnterTimestampMonotonic",
	)
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return nil, fmt.Errorf("systemctl show %s: %w: %s", unit, err, string(exitErr.Stderr))
	}
	return nil, fmt.Errorf("systemctl show %s: %w", unit, err)
}

type systemdStatusReader struct {
	runner systemctlShowRunner
}

func (r systemdStatusReader) Status(ctx context.Context, unit string) (systemd.UnitStatus, error) {
	data, err := r.runner.Show(ctx, unit)
	if err != nil {
		return systemd.UnitStatus{}, err
	}
	status, err := systemd.ParseStatus(data)
	if err != nil {
		return systemd.UnitStatus{}, err
	}
	if status.Name == "" {
		status.Name = unit
	}
	return status, nil
}

type systemdCgroupReader struct {
	fsys fs.FS
}

func (r systemdCgroupReader) ReadUsage(controlGroup string) (systemd.Usage, error) {
	return systemd.ReadUsage(r.fsys, controlGroup)
}

type Option func(*App)

func WithFS(fsys fs.FS) Option {
	return func(a *App) {
		a.fsys = fsys
	}
}

func WithHostCollector(c hostCollector) Option {
	return func(a *App) {
		a.hostCollector = c
	}
}

func WithSystemdStatusReader(r statusReader) Option {
	return func(a *App) {
		a.statusReader = r
	}
}

func WithCgroupUsageReader(r cgroupUsageReader) Option {
	return func(a *App) {
		a.cgroupReader = r
	}
}

func WithPusher(p pusher) Option {
	return func(a *App) {
		a.pusher = p
	}
}

func WithMonotonicNow(f monotonicNowFunc) Option {
	return func(a *App) {
		a.monotonicNowUS = f
	}
}

func WithTickerFactory(factory func(time.Duration) ticker) Option {
	return func(a *App) {
		a.newTicker = factory
	}
}

func WithLogger(l *log.Logger) Option {
	return func(a *App) {
		a.logger = l
	}
}

// App owns sysmon runtime wiring.
type App struct {
	cfg  *config.Config
	host string

	fsys fs.FS

	hostCollector hostCollector
	statusReader  statusReader
	cgroupReader  cgroupUsageReader
	exporter      *metrics.Exporter
	pusher        pusher

	monotonicNowUS monotonicNowFunc
	newTicker      func(time.Duration) ticker
	logger         *log.Logger

	lastServices map[string]metrics.ServiceSample
}

func New(cfg *config.Config, lookupHost func() (string, error), opts ...Option) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if lookupHost == nil {
		return nil, fmt.Errorf("host lookup cannot be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	host, err := identity.ResolveHost(cfg.Identity.Host, lookupHost)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:            cfg,
		host:           host,
		fsys:           os.DirFS("/"),
		exporter:       metrics.NewExporter(),
		pusher:         push.New(cfg.Push.URL, cfg.Push.Job),
		monotonicNowUS: defaultMonotonicNowUS,
		newTicker:      func(d time.Duration) ticker { return timeTicker{t: time.NewTicker(d)} },
		logger:         log.New(os.Stderr, "sysmon: ", log.LstdFlags),
		lastServices:   make(map[string]metrics.ServiceSample),
		statusReader:   systemdStatusReader{runner: systemctlRunner{}},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(app)
		}
	}

	if app.hostCollector == nil {
		app.hostCollector = collector.NewHostCollector(app.fsys, collector.WithCollectPerCoreCPU(cfg.System.CollectPerCoreCPU))
	}
	if app.cgroupReader == nil {
		app.cgroupReader = systemdCgroupReader{fsys: app.fsys}
	}
	if app.statusReader == nil {
		app.statusReader = systemdStatusReader{runner: systemctlRunner{}}
	}
	if app.exporter == nil {
		app.exporter = metrics.NewExporter()
	}
	if app.pusher == nil {
		app.pusher = push.New(cfg.Push.URL, cfg.Push.Job)
	}
	if app.monotonicNowUS == nil {
		app.monotonicNowUS = defaultMonotonicNowUS
	}
	if app.newTicker == nil {
		app.newTicker = func(d time.Duration) ticker { return timeTicker{t: time.NewTicker(d)} }
	}
	if app.logger == nil {
		app.logger = log.New(io.Discard, "", 0)
	}

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}
	if a.cfg == nil || a.exporter == nil || a.pusher == nil {
		return fmt.Errorf("app not initialized")
	}

	t := a.newTicker(a.cfg.Push.Interval.Duration)
	defer t.Stop()

	// Run immediately on startup so a freshly started daemon pushes without
	// waiting a full interval.
	a.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C():
			a.runOnce(ctx)
		}
	}
}

func (a *App) runOnce(ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, a.cfg.Push.Timeout.Duration)
	defer cancel()

	hostSample, err := a.hostCollector.Snapshot(cycleCtx)
	if err != nil {
		a.logger.Printf("host collection failed: %v", err)
		return
	}
	a.exporter.UpdateHost(a.host, hostSample)

	nowUS, err := a.monotonicNowUS()
	if err != nil {
		a.logger.Printf("monotonic now failed: %v", err)
		nowUS = 0
	}

	serviceNames := a.cfg.ServiceNames()
	serviceSamples := make([]metrics.ServiceSample, 0, len(serviceNames))
	for _, name := range serviceNames {
		sample, ok := a.lastServices[name]
		if !ok {
			sample = metrics.ServiceSample{Name: name}
		}

		status, err := a.statusReader.Status(cycleCtx, name)
		if err != nil {
			a.logger.Printf("systemd status failed service=%s: %v", name, err)
			serviceSamples = append(serviceSamples, sample)
			continue
		}

		sample.Name = name
		sample.State = status.State
		sample.Active = status.Active
		sample.Enabled = status.Enabled
		sample.UptimeSeconds = status.ActiveUptimeSeconds(nowUS)

		if status.ControlGroup != "" {
			usage, err := a.cgroupReader.ReadUsage(status.ControlGroup)
			if err != nil {
				a.logger.Printf("cgroup usage failed service=%s control_group=%s: %v", name, status.ControlGroup, err)
			} else {
				sample.CPUUsageSecondsTotal = usage.CPUUsageSecondsTotal
				sample.MemoryResidentBytes = usage.MemoryResidentBytes
			}
		}

		a.lastServices[name] = sample
		serviceSamples = append(serviceSamples, sample)
	}
	a.exporter.UpdateServices(a.host, serviceSamples)

	if err := a.pusher.Push(cycleCtx, a.host, a.exporter); err != nil {
		a.logger.Printf("push failed: %v", err)
	}
}
