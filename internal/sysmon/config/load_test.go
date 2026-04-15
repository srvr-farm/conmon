package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load("testdata/valid.yml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.Push.Job, "sysmon"; got != want {
		t.Fatalf("Push.Job = %q, want %q", got, want)
	}
	if got, want := cfg.ServiceNames(), []string{"docker.service", "sshd.service"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ServiceNames() = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsMissingPushURL(t *testing.T) {
	_, err := Load("testdata/invalid-missing-push-url.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "push.url") {
		t.Fatalf("error = %v, want push.url error", err)
	}
}

func TestLoadRejectsDuplicateServices(t *testing.T) {
	_, err := Load("testdata/invalid-duplicate-service.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error = %v, want duplicate-service error", err)
	}
}

func TestLoadDefaultsCollectPerCoreCPU(t *testing.T) {
	cfg, err := Load("testdata/valid-no-system.yml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.System.CollectPerCoreCPU {
		t.Fatalf("CollectPerCoreCPU = %v, want true", cfg.System.CollectPerCoreCPU)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	_, err := Load("testdata/invalid-unknown-field.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("error = %v, want unknown field error", err)
	}
}

func TestLoadRejectsNonHTTPURL(t *testing.T) {
	_, err := Load("testdata/invalid-push-url-scheme.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "scheme must be http") {
		t.Fatalf("error = %v, want http scheme error", err)
	}
}

func TestLoadRejectsZeroInterval(t *testing.T) {
	_, err := Load("testdata/invalid-interval.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "push.interval must be greater than zero") {
		t.Fatalf("error = %v, want interval error", err)
	}
}

func TestLoadRejectsZeroTimeout(t *testing.T) {
	_, err := Load("testdata/invalid-timeout.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "push.timeout must be greater than zero") {
		t.Fatalf("error = %v, want timeout error", err)
	}
}

func TestLoadRejectsBlankServiceName(t *testing.T) {
	_, err := Load("testdata/invalid-service-name.yml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("error = %v, want service name error", err)
	}
}

func TestIdentityHostIsTrimmed(t *testing.T) {
	cfg, err := Load("testdata/valid-identity-host-trim.yml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.Identity.Host, "trimmed-host"; got != want {
		t.Fatalf("Identity.Host = %q, want %q", got, want)
	}
}
