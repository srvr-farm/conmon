package httpprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/config"
)

func TestProbeRunSuccess(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	check := config.Check{
		ID:             "web",
		Name:           "Public Web",
		Group:          "internet",
		Kind:           "https",
		Scope:          "external",
		URL:            server.URL,
		Method:         http.MethodPost,
		ExpectedStatus: []int{http.StatusNoContent},
		Headers: map[string]string{
			"X-Test": "value",
		},
		Timeout: config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{Client: server.Client()})
	result := probe.Run(context.Background())

	if !result.Success {
		t.Fatal("expected success")
	}
	if got, want := result.HTTPStatusCode, http.StatusNoContent; got != want {
		t.Fatalf("HTTPStatusCode = %d, want %d", got, want)
	}
	if got, want := result.CheckID, check.ID; got != want {
		t.Fatalf("CheckID = %q, want %q", got, want)
	}
	if got, want := result.CheckName, check.Name; got != want {
		t.Fatalf("CheckName = %q, want %q", got, want)
	}
	if got, want := result.CheckGroup, check.Group; got != want {
		t.Fatalf("CheckGroup = %q, want %q", got, want)
	}
	if got, want := result.CheckKind, check.Kind; got != want {
		t.Fatalf("CheckKind = %q, want %q", got, want)
	}
	if got, want := result.CheckScope, check.Scope; got != want {
		t.Fatalf("CheckScope = %q, want %q", got, want)
	}

	select {
	case request := <-requests:
		if got, want := request.Method, http.MethodPost; got != want {
			t.Fatalf("request method = %q, want %q", got, want)
		}
		if got, want := request.Header.Get("X-Test"), "value"; got != want {
			t.Fatalf("request header = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for request")
	}
}

func TestProbeRunFailsWhenStatusIsNotExpected(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	check := config.Check{
		ID:             "web",
		Name:           "Public Web",
		Group:          "internet",
		Kind:           "https",
		Scope:          "external",
		URL:            server.URL,
		Method:         http.MethodGet,
		ExpectedStatus: []int{http.StatusOK},
		Timeout:        config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{Client: server.Client()})
	result := probe.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure")
	}
	if got, want := result.HTTPStatusCode, http.StatusNoContent; got != want {
		t.Fatalf("HTTPStatusCode = %d, want %d", got, want)
	}
}

func TestProbeRunDefaultsExpectedStatusTo200(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	check := config.Check{
		ID:      "web",
		Name:    "Public Web",
		Group:   "internet",
		Kind:    "https",
		Scope:   "external",
		URL:     server.URL,
		Method:  http.MethodGet,
		Timeout: config.Duration{Duration: time.Second},
	}

	probe := New(check, Options{Client: server.Client()})
	result := probe.Run(context.Background())

	if !result.Success {
		t.Fatal("expected success")
	}
	if got, want := result.HTTPStatusCode, http.StatusOK; got != want {
		t.Fatalf("HTTPStatusCode = %d, want %d", got, want)
	}
}
