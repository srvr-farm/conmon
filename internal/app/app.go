package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/metrics"
	"github.com/mcallan/conmon/internal/probe"
	dnsprobe "github.com/mcallan/conmon/internal/probe/dns"
	httpprobe "github.com/mcallan/conmon/internal/probe/http"
	icmpprobe "github.com/mcallan/conmon/internal/probe/icmp"
	tlsprobe "github.com/mcallan/conmon/internal/probe/tls"
	"github.com/mcallan/conmon/internal/scheduler"
)

var newHTTPProbe = func(check config.Check) probe.Probe {
	return httpprobe.New(check, httpprobe.Options{})
}

var newTLSProbe = func(check config.Check) probe.Probe {
	return tlsprobe.New(check, tlsprobe.Options{})
}

var newICMPProbe = func(check config.Check) probe.Probe {
	return icmpprobe.New(check, icmpprobe.Options{})
}

var newDNSProbe = func(check config.Check) probe.Probe {
	return dnsprobe.New(check, dnsprobe.Options{})
}

type schedulerRunner interface {
	Start(ctx context.Context, jobs []scheduler.Job) <-chan struct{}
}

var newScheduler = func() schedulerRunner {
	return scheduler.New(nil)
}

type App struct {
	ConfigPath string

	config    *config.Config
	exporter  *metrics.Exporter
	scheduler schedulerRunner
	jobs      []scheduler.Job
	server    *http.Server
}

func New(configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	checks, err := runtimeChecks(cfg)
	if err != nil {
		return nil, err
	}

	exporter, err := metrics.New(collectLabelKeys(checks))
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter.Handler())

	return &App{
		ConfigPath: configPath,
		config:     cfg,
		exporter:   exporter,
		scheduler:  newScheduler(),
		jobs:       buildJobs(checks, exporter),
		server: &http.Server{
			Addr:    cfg.Export.ListenAddress,
			Handler: mux,
		},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	schedulerDone := a.scheduler.Start(runCtx, a.jobs)
	serverErr := make(chan error, 1)

	go func() {
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		cancel()
		<-schedulerDone
		return err
	case <-ctx.Done():
	}

	cancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := a.server.Shutdown(shutdownCtx)
	<-schedulerDone
	listenErr := <-serverErr

	if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
		return shutdownErr
	}
	return listenErr
}

func runtimeChecks(cfg *config.Config) ([]config.Check, error) {
	var checks []config.Check
	for _, group := range cfg.Groups {
		for _, groupCheck := range group.Checks {
			check := groupCheck
			switch check.Kind {
			case "https":
			case "dns":
			case "icmp":
			case "tls":
			default:
				return nil, fmt.Errorf("check %q uses unsupported probe kind %q; only https, tls, icmp, and dns are implemented", check.ID, check.Kind)
			}
			if check.Kind == "tls" && check.MinDaysRemaining == 0 {
				check.MinDaysRemaining = cfg.Defaults.TLS.MinDaysRemaining
			}
			if check.Interval.Duration <= 0 {
				return nil, fmt.Errorf("check %q interval must be greater than zero", check.ID)
			}
			if check.Timeout.Duration <= 0 {
				return nil, fmt.Errorf("check %q timeout must be greater than zero", check.ID)
			}
			checks = append(checks, check)
		}
	}
	return checks, nil
}

func collectLabelKeys(checks []config.Check) []string {
	seen := make(map[string]struct{})
	var keys []string
	for _, check := range checks {
		for key := range check.Labels {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func buildJobs(checks []config.Check, exporter *metrics.Exporter) []scheduler.Job {
	jobs := make([]scheduler.Job, 0, len(checks))
	for _, check := range checks {
		check := check
		var runner probe.Probe
		switch check.Kind {
		case "https":
			runner = newHTTPProbe(check)
		case "dns":
			runner = newDNSProbe(check)
		case "icmp":
			runner = newICMPProbe(check)
		case "tls":
			runner = newTLSProbe(check)
		}

		jobs = append(jobs, scheduler.Job{
			Interval: check.Interval.Duration,
			Run: func(ctx context.Context) {
				exporter.Record(runner.Run(ctx))
			},
		})
	}
	return jobs
}
