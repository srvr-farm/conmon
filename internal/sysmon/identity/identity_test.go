package identity

import "testing"

func TestResolveHostPrefersConfiguredOverride(t *testing.T) {
	got, err := ResolveHost("edge-a", func() (string, error) { return "ignored", nil })
	if err != nil {
		t.Fatalf("ResolveHost returned error: %v", err)
	}
	if got != "edge-a" {
		t.Fatalf("ResolveHost = %q, want %q", got, "edge-a")
	}
}
