package config

import (
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load("testdata/valid.yml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := len(cfg.Groups), 3; got != want {
		t.Fatalf("len(Groups) = %d, want %d", got, want)
	}
	check := findCheck(t, cfg, "internet-gateway-ping")
	if got, want := check.Group, "internet"; got != want {
		t.Fatalf("check.Group = %q, want %q", got, want)
	}
	if got, want := cfg.Defaults.DNS.Server, "1.1.1.1"; got != want {
		t.Fatalf("defaults.dns.server = %q, want %q", got, want)
	}
	defaultStatusCheck := findCheck(t, cfg, "internal-http-default-status")
	if got, want := defaultStatusCheck.ExpectedStatus, []int{200}; !equalInts(got, want) {
		t.Fatalf("default expected_status = %v, want %v", got, want)
	}
	if got, want := defaultStatusCheck.Method, "GET"; got != want {
		t.Fatalf("default method = %q, want %q", got, want)
	}
	if got, want := defaultStatusCheck.Labels["site"], "home-lab"; got != want {
		t.Fatalf("default label site = %q, want %q", got, want)
	}
	if got, want := defaultStatusCheck.Labels["env"], "lab"; got != want {
		t.Fatalf("default label env = %q, want %q", got, want)
	}
	if got, want := defaultStatusCheck.Interval.Duration, 30*time.Second; got != want {
		t.Fatalf("default interval = %v, want %v", got, want)
	}
	if got, want := defaultStatusCheck.Timeout.Duration, 5*time.Second; got != want {
		t.Fatalf("default timeout = %v, want %v", got, want)
	}
	overrides := findCheck(t, cfg, "internet-web")
	if got, want := overrides.Interval.Duration, 15*time.Second; got != want {
		t.Fatalf("override interval = %v, want %v", got, want)
	}
	if got, want := overrides.Timeout.Duration, 3*time.Second; got != want {
		t.Fatalf("override timeout = %v, want %v", got, want)
	}
	if got, want := overrides.Labels["site"], "edge-lab"; got != want {
		t.Fatalf("merged label site = %q, want %q", got, want)
	}
	if got, want := overrides.Labels["env"], "edge"; got != want {
		t.Fatalf("merged label env = %q, want %q", got, want)
	}
	if got, want := overrides.Labels["team"], "network"; got != want {
		t.Fatalf("merged label team = %q, want %q", got, want)
	}
	if _, ok := overrides.Labels[" site "]; ok {
		t.Fatalf("expected trimmed label keys")
	}
	dnsCheck := findCheck(t, cfg, "external-dns")
	if got, want := dnsCheck.Port, 53; got != want {
		t.Fatalf("default dns port = %d, want %d", got, want)
	}
}

func TestLoadRejectsMissingKind(t *testing.T) {
	_, err := Load("testdata/invalid-missing-kind.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsHTTPWithoutURL(t *testing.T) {
	_, err := Load("testdata/invalid-http.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	_, err := Load("testdata/invalid-duplicate-id.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsSchemeMismatch(t *testing.T) {
	_, err := Load("testdata/invalid-scheme.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidDNSRecordType(t *testing.T) {
	_, err := Load("testdata/invalid-dns-record-type.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsMissingListenAddress(t *testing.T) {
	_, err := Load("testdata/invalid-missing-listen.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsMissingDefaults(t *testing.T) {
	_, err := Load("testdata/invalid-missing-defaults.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsListenAddressPortOutOfRange(t *testing.T) {
	_, err := Load("testdata/invalid-listen-port.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsListenAddressPortNonNumeric(t *testing.T) {
	_, err := Load("testdata/invalid-listen-port-nonnumeric.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidHTTPMethod(t *testing.T) {
	_, err := Load("testdata/invalid-method.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidLabelKey(t *testing.T) {
	_, err := Load("testdata/invalid-label-key.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsReservedLabelKey(t *testing.T) {
	_, err := Load("testdata/invalid-label-key-reserved.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsLabelKeyCollision(t *testing.T) {
	_, err := Load("testdata/invalid-label-collision.yml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadPreservesEmptyDNSServer(t *testing.T) {
	cfg, err := Load("testdata/valid-dns-system.yml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	dnsCheck := findCheck(t, cfg, "system-resolver-dns")
	if dnsCheck.Server != "" {
		t.Fatalf("expected empty dns server, got %q", dnsCheck.Server)
	}
}

func findCheck(t *testing.T, cfg *Config, id string) *Check {
	t.Helper()
	for _, group := range cfg.Groups {
		for i := range group.Checks {
			if group.Checks[i].ID == id {
				return &group.Checks[i]
			}
		}
	}
	t.Fatalf("check with id %q not found", id)
	return nil
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
