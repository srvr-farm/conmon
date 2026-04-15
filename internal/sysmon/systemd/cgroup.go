package systemd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	cgroupRoot = "sys/fs/cgroup"
)

// Usage holds the normalized cgroup usage data that sysmon emits.
type Usage struct {
	CPUUsageSecondsTotal float64
	MemoryResidentBytes  uint64
}

// ReadUsage reads CPU and memory usage for a control group by preferring cgroup v2
// files and falling back to cgroup v1 controllers when necessary.
func ReadUsage(fsys fs.FS, controlGroup string) (Usage, error) {
	if fsys == nil {
		return Usage{}, fmt.Errorf("fs cannot be nil")
	}
	trimmed := strings.TrimPrefix(controlGroup, "/")
	usage := Usage{}
	cpuSet, memSet := false, false

	v2Dir := filepath.Join(cgroupRoot, trimmed)
	if cpuSeconds, ok, err := tryReadV2CPU(fsys, v2Dir); err != nil {
		return Usage{}, err
	} else if ok {
		usage.CPUUsageSecondsTotal = cpuSeconds
		cpuSet = true
	}
	if memBytes, ok, err := tryReadV2Memory(fsys, v2Dir); err != nil {
		return Usage{}, err
	} else if ok {
		usage.MemoryResidentBytes = memBytes
		memSet = true
	}

	if !cpuSet {
		if cpuSeconds, ok, err := tryReadV1CPU(fsys, trimmed); err != nil {
			return Usage{}, err
		} else if ok {
			usage.CPUUsageSecondsTotal = cpuSeconds
			cpuSet = true
		}
	}
	if !memSet {
		if memBytes, ok, err := tryReadV1Memory(fsys, trimmed); err != nil {
			return Usage{}, err
		} else if ok {
			usage.MemoryResidentBytes = memBytes
			memSet = true
		}
	}

	if !cpuSet {
		return Usage{}, fmt.Errorf("cpu usage not available for %q", controlGroup)
	}
	if !memSet {
		return Usage{}, fmt.Errorf("memory usage not available for %q", controlGroup)
	}
	return usage, nil
}

func tryReadV2CPU(fsys fs.FS, dir string) (float64, bool, error) {
	path := filepath.Join(dir, "cpu.stat")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read cpu.stat: %w", err)
	}
	cpuSeconds, err := parseCPUStatUsage(data)
	if err != nil {
		return 0, false, err
	}
	return cpuSeconds, true, nil
}

func tryReadV2Memory(fsys fs.FS, dir string) (uint64, bool, error) {
	path := filepath.Join(dir, "memory.current")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read memory.current: %w", err)
	}
	memBytes, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse memory.current: %w", err)
	}
	return memBytes, true, nil
}

func tryReadV1CPU(fsys fs.FS, controlGroup string) (float64, bool, error) {
	path := filepath.Join(cgroupRoot, "cpuacct", controlGroup, "cpuacct.usage")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read cpuacct.usage: %w", err)
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse cpuacct.usage: %w", err)
	}
	return float64(v) / 1_000_000_000, true, nil
}

func tryReadV1Memory(fsys fs.FS, controlGroup string) (uint64, bool, error) {
	path := filepath.Join(cgroupRoot, "memory", controlGroup, "memory.usage_in_bytes")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read memory.usage_in_bytes: %w", err)
	}
	memBytes, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse memory.usage_in_bytes: %w", err)
	}
	return memBytes, true, nil
}

func parseCPUStatUsage(data []byte) (float64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 || parts[0] != "usage_usec" {
			continue
		}
		v, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse cpu.stat usage_usec: %w", err)
		}
		return float64(v) / 1_000_000, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("usage_usec not found")
}
