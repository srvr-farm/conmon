package systemd

import (
	"strings"
	"testing"
)

func TestParseShowOutputBuildsUnitStatus(t *testing.T) {
	status, err := ParseStatus([]byte(strings.Join([]string{
		"Id=docker.service",
		"ActiveState=active",
		"UnitFileState=enabled",
		"ActiveEnterTimestampMonotonic=15000000",
		"ControlGroup=/system.slice/docker.service",
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseStatus returned error: %v", err)
	}
	if !status.Active {
		t.Fatalf("Active = false, want true")
	}
	if got, want := status.Enabled, true; got != want {
		t.Fatalf("Enabled = %v, want %v", got, want)
	}
	if got, want := status.ControlGroup, "/system.slice/docker.service"; got != want {
		t.Fatalf("ControlGroup = %q, want %q", got, want)
	}
	if got, want := status.ActiveEnterMonotonicUS, uint64(15000000); got != want {
		t.Fatalf("ActiveEnterMonotonicUS = %d, want %d", got, want)
	}
}

func TestEnabledFromUnitFileState(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		if !EnabledFromUnitFileState("enabled") {
			t.Fatalf("unit should be enabled")
		}
	})

	t.Run("linked", func(t *testing.T) {
		if !EnabledFromUnitFileState("linked") {
			t.Fatalf("unit should be enabled")
		}
	})

	t.Run("masked", func(t *testing.T) {
		if EnabledFromUnitFileState("masked") {
			t.Fatalf("masked should not be enabled")
		}
	})
}

func TestKnownStateValues(t *testing.T) {
	values := KnownStateValues()
	expected := []string{"active", "inactive", "failed", "activating", "deactivating"}
	if len(values) != len(expected) {
		t.Fatalf("KnownStateValues length = %d, want %d", len(values), len(expected))
	}
	for i := range values {
		if values[i] != expected[i] {
			t.Fatalf("value[%d] = %q, want %q", i, values[i], expected[i])
		}
	}
}

func TestComputeUptimeSeconds(t *testing.T) {
	if got := ComputeUptimeSeconds(2_000_000, 1_000_000); got != 1.0 {
		t.Fatalf("ComputeUptimeSeconds = %v, want 1.0", got)
	}
	if got := ComputeUptimeSeconds(1_000_000, 2_000_000); got != 0 {
		t.Fatalf("ComputeUptimeSeconds with earlier now = %v, want 0", got)
	}
	if got := ComputeUptimeSeconds(1_000_000, 0); got != 0 {
		t.Fatalf("ComputeUptimeSeconds with zero enter = %v, want 0", got)
	}
}

func TestActiveUptimeSeconds(t *testing.T) {
	status := UnitStatus{Active: true, ActiveEnterMonotonicUS: 1_000_000}
	if got := status.ActiveUptimeSeconds(2_000_000); got != 1.0 {
		t.Fatalf("ActiveUptimeSeconds = %v, want 1", got)
	}
	status.Active = false
	if got := status.ActiveUptimeSeconds(2_000_000); got != 0 {
		t.Fatalf("ActiveUptimeSeconds when inactive = %v, want 0", got)
	}
}
