package identity

import (
	"errors"
	"testing"
)

func TestResolveHostPrefersConfiguredOverride(t *testing.T) {
	got, err := ResolveHost("edge-a", func() (string, error) { return "ignored", nil })
	if err != nil {
		t.Fatalf("ResolveHost returned error: %v", err)
	}
	if got != "edge-a" {
		t.Fatalf("ResolveHost = %q, want %q", got, "edge-a")
	}
}

func TestResolveHostPropagatesLookupError(t *testing.T) {
	wantErr := errors.New("lookup failure")
	if _, err := ResolveHost("", func() (string, error) { return "", wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("ResolveHost error = %v, want %v", err, wantErr)
	}
}
