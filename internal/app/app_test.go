package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/probe"
	httpprobe "github.com/mcallan/conmon/internal/probe/http"
	tlsprobe "github.com/mcallan/conmon/internal/probe/tls"
	"github.com/mcallan/conmon/internal/scheduler"
)

func TestNewRejectsUnsupportedProbeKinds(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		kind          string
		extra         string
		errorFragment string
	}{
		{
			name:          "http",
			kind:          "http",
			extra:         "url: http://127.0.0.1/health",
			errorFragment: `unsupported probe kind "http"`,
		},
		{
			name:          "tcp",
			kind:          "tcp",
			extra:         "host: 127.0.0.1\n        port: 443",
			errorFragment: `unsupported probe kind "tcp"`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := writeConfig(t, `
defaults:
  interval: 50ms
  timeout: 1s
  tls:
    min_days_remaining: 7
groups:
  - name: integration
    checks:
      - id: unsupported
        name: Unsupported
        kind: `+testCase.kind+`
        scope: internal
        `+testCase.extra+`
export:
  listen_address: 127.0.0.1:9109
`)

			_, err := New(configPath)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), testCase.errorFragment) {
				t.Fatalf("error = %v, want unsupported-kind error", err)
			}
		})
	}
}

func TestNewAcceptsImplementedProbeKinds(t *testing.T) {
	configPath := writeConfig(t, `
defaults:
  interval: 50ms
  timeout: 1s
  dns:
    server: 10.0.0.1
  tls:
    min_days_remaining: 7
groups:
  - name: integration
    checks:
      - id: web
        name: Web
        kind: https
        scope: external
        url: https://callanarchitects.com/
      - id: cert
        name: Certificate
        kind: tls
        scope: external
        host: callanarchitects.com
        port: 443
      - id: public-ping
        name: Public Ping
        kind: icmp
        scope: external
        host: 8.8.8.8
      - id: local-dns
        name: Local DNS
        kind: dns
        scope: internal
        query_name: callanarchitects.com
        record_type: A
        expected_rcode: NOERROR
export:
  listen_address: 127.0.0.1:9109
`)

	app, err := New(configPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if got, want := len(app.jobs), 4; got != want {
		t.Fatalf("len(app.jobs) = %d, want %d", got, want)
	}
}

func TestRunServesMetricsForSupportedChecks(t *testing.T) {
	probeServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Fatalf("request method = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer probeServer.Close()

	originalHTTPProbeFactory := newHTTPProbe
	originalTLSProbeFactory := newTLSProbe
	newHTTPProbe = func(check config.Check) probe.Probe {
		return httpprobe.New(check, httpprobe.Options{Client: probeServer.Client()})
	}
	rootCAs := probeServer.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	newTLSProbe = func(check config.Check) probe.Probe {
		return tlsprobe.New(check, tlsprobe.Options{RootCAs: rootCAs})
	}
	defer func() {
		newHTTPProbe = originalHTTPProbeFactory
		newTLSProbe = originalTLSProbeFactory
	}()

	host, port := splitHostPort(t, probeServer.Listener.Addr().String())
	listenAddress := reserveListenAddress(t)
	configPath := writeConfig(t, `
defaults:
  interval: 50ms
  timeout: 1s
  tls:
    min_days_remaining: 1
groups:
  - name: integration
    checks:
      - id: web
        name: Web
        kind: https
        scope: external
        url: `+probeServer.URL+`
        expected_status: [204]
      - id: cert
        name: Certificate
        kind: tls
        scope: external
        host: `+host+`
        port: `+portString(port)+`
export:
  listen_address: `+listenAddress+`
`)

	app, err := New(configPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(ctx)
	}()

	metricsURL := "http://" + listenAddress + "/metrics"
	body := waitForMetrics(t, metricsURL,
		`conmon_probe_runs_total{check_id="web"`,
		`conmon_probe_runs_total{check_id="cert"`,
		`conmon_http_status_code{check_id="web"`,
		`conmon_tls_cert_days_remaining{check_id="cert"`,
	)

	if !strings.Contains(body, `conmon_http_status_code{check_id="web",check_name="Web",group="integration",kind="https",scope="external"} 204`) {
		t.Fatalf("metrics body missing http status sample:\n%s", body)
	}
	if !strings.Contains(body, `conmon_probe_success{check_id="cert",check_name="Certificate",group="integration",kind="tls",scope="external"} 1`) {
		t.Fatalf("metrics body missing tls success sample:\n%s", body)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for app shutdown")
	}
}

func TestRuntimeChecksRejectNonPositiveIntervals(t *testing.T) {
	_, err := runtimeChecks(&config.Config{
		Defaults: config.Defaults{
			TLS: config.TLSDefaults{MinDaysRemaining: 7},
		},
		Groups: []config.Group{
			{
				Name: "integration",
				Checks: []config.Check{
					{
						ID:       "web",
						Name:     "Web",
						Group:    "integration",
						Kind:     "https",
						Scope:    "external",
						URL:      "https://example.com/health",
						Interval: config.Duration{},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `interval must be greater than zero`) {
		t.Fatalf("error = %v, want interval error", err)
	}
}

func TestRuntimeChecksRejectNonPositiveTimeouts(t *testing.T) {
	_, err := runtimeChecks(&config.Config{
		Defaults: config.Defaults{
			TLS: config.TLSDefaults{MinDaysRemaining: 7},
		},
		Groups: []config.Group{
			{
				Name: "integration",
				Checks: []config.Check{
					{
						ID:       "web",
						Name:     "Web",
						Group:    "integration",
						Kind:     "https",
						Scope:    "external",
						URL:      "https://example.com/health",
						Interval: config.Duration{Duration: time.Second},
						Timeout:  config.Duration{},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `timeout must be greater than zero`) {
		t.Fatalf("error = %v, want timeout error", err)
	}
}

func TestRunCancelsSchedulerWhenListenFailsImmediately(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen returned error: %v", err)
	}
	defer occupied.Close()

	configPath := writeConfig(t, `
defaults:
  interval: 50ms
  timeout: 1s
  tls:
    min_days_remaining: 1
groups:
  - name: integration
    checks:
      - id: cert
        name: Certificate
        kind: tls
        scope: external
        host: 127.0.0.1
        port: 443
export:
  listen_address: `+occupied.Addr().String()+`
`)

	originalSchedulerFactory := newScheduler
	fake := &fakeScheduler{
		done: make(chan struct{}),
	}
	newScheduler = func() schedulerRunner {
		return fake
	}
	defer func() {
		newScheduler = originalSchedulerFactory
	}()

	app, err := New(configPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	runErr := app.Run(context.Background())
	if runErr == nil {
		t.Fatal("expected listen error")
	}
	if !strings.Contains(runErr.Error(), "address already in use") {
		t.Fatalf("Run error = %v, want listen failure", runErr)
	}
	select {
	case <-fake.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler context was not canceled")
	}
	select {
	case <-fake.done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not finish")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "conmon-*.yml")
	if err != nil {
		t.Fatalf("os.CreateTemp returned error: %v", err)
	}
	if _, err := file.WriteString(strings.TrimSpace(contents) + "\n"); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	return file.Name()
}

func reserveListenAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen returned error: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	return address
}

func waitForMetrics(t *testing.T, url string, fragments ...string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil {
				text := string(body)
				if hasAllFragments(text, fragments) {
					return text
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for metrics at %s", url)
	return ""
}

func hasAllFragments(text string, fragments []string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			return false
		}
	}
	return true
}

func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("net.SplitHostPort returned error: %v", err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatalf("net.LookupPort returned error: %v", err)
	}
	return host, port
}

func portString(port int) string {
	return strconv.Itoa(port)
}

type fakeScheduler struct {
	canceled chan struct{}
	done     chan struct{}
}

func (f *fakeScheduler) Start(ctx context.Context, jobs []scheduler.Job) <-chan struct{} {
	if len(jobs) == 0 {
		close(f.done)
		return f.done
	}
	f.canceled = make(chan struct{})
	go func() {
		<-ctx.Done()
		close(f.canceled)
		close(f.done)
	}()
	return f.done
}
