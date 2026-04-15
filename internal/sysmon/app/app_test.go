package app

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/sysmon/collector"
	"github.com/mcallan/conmon/internal/sysmon/config"
	"github.com/mcallan/conmon/internal/sysmon/systemd"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewBuildsAppWithResolvedHost(t *testing.T) {
	cfg := &config.Config{
		Push: config.PushConfig{
			URL:      "http://127.0.0.1:9092",
			Job:      "sysmon",
			Interval: config.Duration{Duration: 30 * time.Second},
			Timeout:  config.Duration{Duration: 5 * time.Second},
		},
	}

	app, err := New(cfg, func() (string, error) { return "edge-a", nil })
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if got, want := app.host, "edge-a"; got != want {
		t.Fatalf("host = %q, want %q", got, want)
	}
}

type fakeHostCollector struct {
	sample collector.HostSample
	err    error
}

func (f *fakeHostCollector) Snapshot(ctx context.Context) (collector.HostSample, error) {
	return f.sample, f.err
}

type sequenceHostCollector struct {
	samples []collector.HostSample
	errs    []error
	mu      sync.Mutex
	calls   int
}

func (c *sequenceHostCollector) Snapshot(ctx context.Context) (collector.HostSample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.calls
	c.calls++
	var sample collector.HostSample
	var err error
	if i < len(c.samples) {
		sample = c.samples[i]
	}
	if i < len(c.errs) {
		err = c.errs[i]
	}
	return sample, err
}

type fakeStatusReader struct {
	statusByUnit map[string]systemd.UnitStatus
	errByUnit    map[string]error
}

func (r fakeStatusReader) Status(ctx context.Context, unit string) (systemd.UnitStatus, error) {
	if err := r.errByUnit[unit]; err != nil {
		return systemd.UnitStatus{}, err
	}
	if status, ok := r.statusByUnit[unit]; ok {
		return status, nil
	}
	return systemd.UnitStatus{}, nil
}

type fakeCgroupReader struct {
	usageByGroup map[string]systemd.Usage
	errByGroup   map[string]error
}

func (r fakeCgroupReader) ReadUsage(controlGroup string) (systemd.Usage, error) {
	if err := r.errByGroup[controlGroup]; err != nil {
		return systemd.Usage{}, err
	}
	if usage, ok := r.usageByGroup[controlGroup]; ok {
		return usage, nil
	}
	return systemd.Usage{}, nil
}

type mutableCgroupReader struct {
	mu           sync.Mutex
	usageByGroup map[string]systemd.Usage
	errByGroup   map[string]error
}

func (r *mutableCgroupReader) ReadUsage(controlGroup string) (systemd.Usage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.errByGroup[controlGroup]; err != nil {
		return systemd.Usage{}, err
	}
	if usage, ok := r.usageByGroup[controlGroup]; ok {
		return usage, nil
	}
	return systemd.Usage{}, nil
}

type fakeTicker struct {
	ch chan time.Time
}

func (t fakeTicker) C() <-chan time.Time { return t.ch }
func (t fakeTicker) Stop()               {}

type pushCall struct {
	host     string
	families []*dto.MetricFamily
}

type fakePusher struct {
	mu    sync.Mutex
	calls []pushCall
	done  chan struct{}
	callC chan struct{}
}

func (p *fakePusher) Push(ctx context.Context, host string, reg prometheus.Gatherer) error {
	families, err := reg.Gather()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.calls = append(p.calls, pushCall{host: host, families: families})
	first := len(p.calls) == 1
	p.mu.Unlock()
	if first && p.done != nil {
		close(p.done)
	}
	if p.callC != nil {
		p.callC <- struct{}{}
	}
	return nil
}

func TestRunPushesRegistryAndStopsOnCancel(t *testing.T) {
	cfg := &config.Config{
		Push: config.PushConfig{
			URL:      "http://127.0.0.1:9092",
			Job:      "sysmon",
			Interval: config.Duration{Duration: 30 * time.Second},
			Timeout:  config.Duration{Duration: 5 * time.Second},
		},
		Services: []config.Service{{Name: "demo.service"}},
	}

	pusher := &fakePusher{done: make(chan struct{})}
	ft := fakeTicker{ch: make(chan time.Time)}

	instance, err := New(cfg, func() (string, error) { return "edge-a", nil },
		WithHostCollector(&fakeHostCollector{sample: collector.HostSample{
			UptimeSeconds:       100,
			BootID:              "boot-a",
			MemoryResidentBytes: 256,
			TotalCPUUsageRatio:  0.5,
		}}),
		WithSystemdStatusReader(fakeStatusReader{
			statusByUnit: map[string]systemd.UnitStatus{
				"demo.service": {
					Name:                   "demo.service",
					State:                  "active",
					Active:                 true,
					Enabled:                true,
					ControlGroup:           "/system.slice/demo.service",
					ActiveEnterMonotonicUS: 1_000_000,
				},
			},
		}),
		WithCgroupUsageReader(fakeCgroupReader{
			usageByGroup: map[string]systemd.Usage{
				"/system.slice/demo.service": {
					CPUUsageSecondsTotal: 12.5,
					MemoryResidentBytes:  1024,
				},
			},
		}),
		WithMonotonicNow(func() (uint64, error) { return 4_000_000, nil }),
		WithPusher(pusher),
		WithTickerFactory(func(d time.Duration) ticker { return ft }),
		WithLogger(log.New(io.Discard, "", 0)),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- instance.Run(ctx)
	}()

	select {
	case <-pusher.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first push")
	}
	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Run to return")
	}

	pusher.mu.Lock()
	if got, want := len(pusher.calls), 1; got != want {
		pusher.mu.Unlock()
		t.Fatalf("push calls = %d, want %d", got, want)
	}
	call := pusher.calls[0]
	pusher.mu.Unlock()

	if got, want := call.host, "edge-a"; got != want {
		t.Fatalf("pushed host = %q, want %q", got, want)
	}

	if got, want := findGauge(call.families, "sysmon_host_uptime_seconds", map[string]string{"host": "edge-a"}), 100.0; got != want {
		t.Fatalf("host uptime = %v, want %v", got, want)
	}
	if got, want := findGauge(call.families, "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "demo.service"}), 3.0; got != want {
		t.Fatalf("service uptime = %v, want %v", got, want)
	}
	if got, want := findGauge(call.families, "sysmon_service_active", map[string]string{"host": "edge-a", "service": "demo.service"}), 1.0; got != want {
		t.Fatalf("service active = %v, want %v", got, want)
	}
	if got, want := findGauge(call.families, "sysmon_service_cpu_usage_seconds_total", map[string]string{"host": "edge-a", "service": "demo.service"}), 12.5; got != want {
		t.Fatalf("service cpu = %v, want %v", got, want)
	}
	if got, want := findGauge(call.families, "sysmon_service_memory_resident_bytes", map[string]string{"host": "edge-a", "service": "demo.service"}), 1024.0; got != want {
		t.Fatalf("service memory = %v, want %v", got, want)
	}
}

func TestRunReturnsErrorOnFirstCycleFailure(t *testing.T) {
	cfg := &config.Config{
		Push: config.PushConfig{
			URL:      "http://127.0.0.1:9092",
			Job:      "sysmon",
			Interval: config.Duration{Duration: 30 * time.Second},
			Timeout:  config.Duration{Duration: 5 * time.Second},
		},
	}

	instance, err := New(cfg, func() (string, error) { return "edge-a", nil },
		WithHostCollector(&fakeHostCollector{err: context.DeadlineExceeded}),
		WithPusher(&fakePusher{}),
		WithTickerFactory(func(d time.Duration) ticker { return fakeTicker{ch: make(chan time.Time)} }),
		WithLogger(log.New(io.Discard, "", 0)),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := instance.Run(ctx); err == nil {
		t.Fatal("Run returned nil error, want non-nil")
	}
}

func TestRunClearsServiceResourcesWhenCgroupReadFailsAfterSuccess(t *testing.T) {
	cfg := &config.Config{
		Push: config.PushConfig{
			URL:      "http://127.0.0.1:9092",
			Job:      "sysmon",
			Interval: config.Duration{Duration: 30 * time.Second},
			Timeout:  config.Duration{Duration: 5 * time.Second},
		},
		Services: []config.Service{{Name: "demo.service"}},
	}

	status := systemd.UnitStatus{
		Name:                   "demo.service",
		State:                  "active",
		Active:                 true,
		Enabled:                true,
		ControlGroup:           "/system.slice/demo.service",
		ActiveEnterMonotonicUS: 1_000_000,
	}

	host := &sequenceHostCollector{
		samples: []collector.HostSample{
			{UptimeSeconds: 100, BootID: "boot-a", MemoryResidentBytes: 256, TotalCPUUsageRatio: 0.5},
			{UptimeSeconds: 101, BootID: "boot-a", MemoryResidentBytes: 300, TotalCPUUsageRatio: 0.6},
		},
	}

	pusher := &fakePusher{callC: make(chan struct{}, 4)}
	ft := fakeTicker{ch: make(chan time.Time, 4)}
	cgroup := &mutableCgroupReader{
		usageByGroup: map[string]systemd.Usage{
			"/system.slice/demo.service": {CPUUsageSecondsTotal: 12.5, MemoryResidentBytes: 1024},
		},
	}

	instance, err := New(cfg, func() (string, error) { return "edge-a", nil },
		WithHostCollector(host),
		WithSystemdStatusReader(fakeStatusReader{statusByUnit: map[string]systemd.UnitStatus{"demo.service": status}}),
		WithCgroupUsageReader(cgroup),
		WithMonotonicNow(func() (uint64, error) { return 4_000_000, nil }),
		WithPusher(pusher),
		WithTickerFactory(func(d time.Duration) ticker { return ft }),
		WithLogger(log.New(io.Discard, "", 0)),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- instance.Run(ctx)
	}()

	// Wait for the startup push.
	select {
	case <-pusher.callC:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first push")
	}

	// Fail cgroup read for the second cycle and trigger it.
	cgroup.mu.Lock()
	cgroup.errByGroup = map[string]error{"/system.slice/demo.service": io.ErrUnexpectedEOF}
	cgroup.mu.Unlock()
	ft.ch <- time.Now()

	select {
	case <-pusher.callC:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second push")
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Run to return")
	}

	pusher.mu.Lock()
	if len(pusher.calls) < 2 {
		pusher.mu.Unlock()
		t.Fatalf("push calls = %d, want >= 2", len(pusher.calls))
	}
	first := pusher.calls[0]
	second := pusher.calls[1]
	pusher.mu.Unlock()

	if got, want := findGauge(first.families, "sysmon_service_cpu_usage_seconds_total", map[string]string{"host": "edge-a", "service": "demo.service"}), 12.5; got != want {
		t.Fatalf("first cycle cpu = %v, want %v", got, want)
	}
	if got, want := findGauge(second.families, "sysmon_service_cpu_usage_seconds_total", map[string]string{"host": "edge-a", "service": "demo.service"}), 0.0; got != want {
		t.Fatalf("second cycle cpu = %v, want %v", got, want)
	}
	if got, want := findGauge(second.families, "sysmon_service_memory_resident_bytes", map[string]string{"host": "edge-a", "service": "demo.service"}), 0.0; got != want {
		t.Fatalf("second cycle mem = %v, want %v", got, want)
	}
}

func findGauge(families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	for _, family := range families {
		if family == nil || family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric == nil {
				continue
			}
			if !labelsMatch(metric.GetLabel(), labels) {
				continue
			}
			if metric.Gauge == nil {
				return 0
			}
			return metric.Gauge.GetValue()
		}
	}
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, labels map[string]string) bool {
	for k, v := range labels {
		found := false
		for _, pair := range pairs {
			if pair.GetName() == k && pair.GetValue() == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
