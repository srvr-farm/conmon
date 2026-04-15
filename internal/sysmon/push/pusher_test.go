package push

import (
    "context"
    "net/http"
    "net/http/httptest"
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
