package systemd

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// UnitStatus represents the relevant fields parsed from `systemctl show`.
type UnitStatus struct {
	Name                   string
	State                  string
	Active                 bool
	Enabled                bool
	ControlGroup           string
	ActiveEnterMonotonicUS uint64
}

// ParseStatus normalizes the subset of `systemctl show` fields that sysmon cares
// about.
func ParseStatus(data []byte) (UnitStatus, error) {
	var status UnitStatus
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := strings.TrimSpace(parts[1])
		switch key {
		case "Id":
			status.Name = value
		case "ActiveState":
			status.State = value
		case "UnitFileState":
			status.Enabled = EnabledFromUnitFileState(value)
		case "ActiveEnterTimestampMonotonic":
			if value == "" {
				continue
			}
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return UnitStatus{}, fmt.Errorf("parse %s: %w", key, err)
			}
			status.ActiveEnterMonotonicUS = v
		case "ControlGroup":
			status.ControlGroup = value
		}
	}
	if err := scanner.Err(); err != nil {
		return UnitStatus{}, err
	}
	status.Active = isActiveState(status.State)
	return status, nil
}

// KnownStateValues returns the set of stable states sysmon exposes.
func KnownStateValues() []string {
	return []string{"active", "inactive", "failed", "activating", "deactivating"}
}

// EnabledFromUnitFileState reports whether a unit is enabled from the unit file
// state reported by systemctl.
func EnabledFromUnitFileState(state string) bool {
	switch strings.TrimSpace(strings.ToLower(state)) {
	case "enabled", "linked":
		return true
	default:
		return false
	}
}

// ComputeUptimeSeconds derives the service uptime from monotonic timestamps
// reported by systemd.
func ComputeUptimeSeconds(nowMonotonicUS, enterMonotonicUS uint64) float64 {
	if nowMonotonicUS <= enterMonotonicUS || enterMonotonicUS == 0 {
		return 0
	}
	return float64(nowMonotonicUS-enterMonotonicUS) / 1_000_000
}

// ActiveUptimeSeconds returns the uptime for an actively running unit, or zero
// otherwise.
func (u UnitStatus) ActiveUptimeSeconds(nowMonotonicUS uint64) float64 {
	if !u.Active {
		return 0
	}
	return ComputeUptimeSeconds(nowMonotonicUS, u.ActiveEnterMonotonicUS)
}

func isActiveState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "active", "activating":
		return true
	default:
		return false
	}
}
