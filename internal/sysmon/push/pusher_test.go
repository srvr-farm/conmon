package push

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPusherUsesJobAndHostGroupingKey(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	err := New(server.URL, "sysmon").Push(context.Background(), "edge-a", prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if got, want := receivedPath, "/metrics/job/sysmon/host/edge-a"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestPusherSucceedsWhenRegistryContainsHostLabel(t *testing.T) {
	var receivedPath string
	var receivedBody string
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		receivedPath = r.URL.Path
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(body): %v", err)
		}
		receivedBody = string(b)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	reg := prometheus.NewRegistry()
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_push_test_metric",
		Help: "test metric for Pushgateway integration",
	}, []string{"host"})
	g.WithLabelValues("edge-a").Set(1)
	reg.MustRegister(g)

	err := New(server.URL, "sysmon").Push(context.Background(), "edge-a", reg)
	if err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if !called {
		t.Fatal("expected Pushgateway handler to be called")
	}
	if got, want := receivedPath, "/metrics/job/sysmon/host/edge-a"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if strings.Contains(receivedBody, `host="edge-a"`) {
		t.Fatalf("expected request body to have host label stripped; body=%q", receivedBody)
	}
	if !strings.Contains(receivedBody, "sysmon_push_test_metric") {
		t.Fatalf("expected request body to contain metric name; body=%q", receivedBody)
	}
}

func TestPusherReturnsErrorOnBadResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := New(server.URL, "sysmon").Push(context.Background(), "edge-a", prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPusherReturnsErrorOnNetworkFailure(t *testing.T) {
	pusher := New("http://localhost:1", "sysmon")
	if err := pusher.Push(context.Background(), "edge-a", prometheus.NewRegistry()); err == nil {
		t.Fatal("expected error when Pushgateway unreachable")
	}
}
