package systemd

import (
	"testing"
	"testing/fstest"
)

func TestReadUsageReadsUnifiedCgroupFiles(t *testing.T) {
	fs := fstest.MapFS{
		"sys/fs/cgroup/system.slice/docker.service/cpu.stat":       {Data: []byte("usage_usec 4200000\n")},
		"sys/fs/cgroup/system.slice/docker.service/memory.current": {Data: []byte("8192\n")},
	}

	usage, err := ReadUsage(fs, "/system.slice/docker.service")
	if err != nil {
		t.Fatalf("ReadUsage returned error: %v", err)
	}
	if got := usage.CPUUsageSecondsTotal; got != 4.2 {
		t.Fatalf("CPUUsageSecondsTotal = %v, want 4.2", got)
	}
	if got := usage.MemoryResidentBytes; got != 8192 {
		t.Fatalf("MemoryResidentBytes = %d, want %d", got, 8192)
	}
}

func TestReadUsageFallsBackToCgroupV1(t *testing.T) {
	fs := fstest.MapFS{
		"sys/fs/cgroup/cpuacct/system.slice/docker.service/cpuacct.usage":        {Data: []byte("4200000000")},
		"sys/fs/cgroup/memory/system.slice/docker.service/memory.usage_in_bytes": {Data: []byte("16384")},
	}

	usage, err := ReadUsage(fs, "/system.slice/docker.service")
	if err != nil {
		t.Fatalf("ReadUsage returned error: %v", err)
	}
	if got := usage.CPUUsageSecondsTotal; got != 4.2 {
		t.Fatalf("CPUUsageSecondsTotal = %v, want 4.2", got)
	}
	if got := usage.MemoryResidentBytes; got != 16384 {
		t.Fatalf("MemoryResidentBytes = %d, want %d", got, 16384)
	}
}

func TestReadUsageCombinesV2AndV1Sources(t *testing.T) {
	fs := fstest.MapFS{
		"sys/fs/cgroup/system.slice/docker.service/cpu.stat":                     {Data: []byte("usage_usec 2100000\n")},
		"sys/fs/cgroup/memory/system.slice/docker.service/memory.usage_in_bytes": {Data: []byte("4096")},
	}

	usage, err := ReadUsage(fs, "/system.slice/docker.service")
	if err != nil {
		t.Fatalf("ReadUsage returned error: %v", err)
	}
	if got := usage.CPUUsageSecondsTotal; got != 2.1 {
		t.Fatalf("CPUUsageSecondsTotal = %v, want 2.1", got)
	}
	if got := usage.MemoryResidentBytes; got != 4096 {
		t.Fatalf("MemoryResidentBytes = %d, want %d", got, 4096)
	}
}

func TestReadUsageMissingCgroupFiles(t *testing.T) {
	fs := fstest.MapFS{
		"sys/fs/cgroup/system.slice/docker.service/memory.current": {Data: []byte("8192")},
	}
	if _, err := ReadUsage(fs, "/system.slice/docker.service"); err == nil {
		t.Fatal("expected error when cpu usage is unavailable")
	}
}
