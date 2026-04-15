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
