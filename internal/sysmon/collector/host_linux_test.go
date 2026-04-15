package collector

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestCollectHostSampleParsesProcData(t *testing.T) {
	fs := fstest.MapFS{
		"proc/stat":                      &fstest.MapFile{Data: []byte("cpu  100 0 50 400 0 0 0 0 0 0\ncpu0 50 0 25 200 0 0 0 0 0 0\ncpu1 50 0 25 200 0 0 0 0 0 0\n")},
		"proc/meminfo":                   &fstest.MapFile{Data: []byte("MemTotal:       1000 kB\nMemAvailable:    250 kB\n")},
		"proc/uptime":                    &fstest.MapFile{Data: []byte("120.50 0.00\n")},
		"proc/sys/kernel/random/boot_id": &fstest.MapFile{Data: []byte("boot-123\n")},
	}

	collector := NewHostCollector(fs)
	sample, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if got, want := sample.MemoryResidentBytes, uint64(768000); got != want {
		t.Fatalf("MemoryResidentBytes = %d, want %d", got, want)
	}
}

func TestCollectHostSampleCPUUsageRatio(t *testing.T) {
	const (
		initial      = "cpu  100 0 50 400 0 0 0 0 0 0\ncpu0 50 0 25 200 0 0 0 0 0 0\ncpu1 50 0 25 200 0 0 0 0 0 0\n"
		second       = "cpu  200 0 100 500 0 0 0 0 0 0\ncpu0 100 0 50 250 0 0 0 0 0 0\ncpu1 100 0 50 250 0 0 0 0 0 0\n"
		meminfoConst = "MemTotal:       1000 kB\nMemAvailable:    250 kB\n"
	)

	buildFS := func(stat string) fstest.MapFS {
		return fstest.MapFS{
			"proc/stat":                      &fstest.MapFile{Data: []byte(stat)},
			"proc/meminfo":                   &fstest.MapFile{Data: []byte(meminfoConst)},
			"proc/uptime":                    &fstest.MapFile{Data: []byte("600.00 0.00\n")},
			"proc/sys/kernel/random/boot_id": &fstest.MapFile{Data: []byte("boot-456\n")},
		}
	}

	ctx := context.Background()
	fs := buildFS(initial)
	collector := NewHostCollector(fs)
	if _, err := collector.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot first call returned error: %v", err)
	}
	fs["proc/stat"] = &fstest.MapFile{Data: []byte(second)}
	sample, err := collector.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot second call returned error: %v", err)
	}
	if sample.TotalCPUUsageRatio <= 0 || sample.TotalCPUUsageRatio >= 1 {
		t.Fatalf("TotalCPUUsageRatio = %f, want between 0 and 1", sample.TotalCPUUsageRatio)
	}
	if len(sample.PerCoreUsageRatio) != 2 {
		t.Fatalf("per-core ratios count = %d, want %d", len(sample.PerCoreUsageRatio), 2)
	}
	for core, ratio := range sample.PerCoreUsageRatio {
		if ratio <= 0 || ratio >= 1 {
			t.Fatalf("Per-core ratio for %s = %f, want between 0 and 1", core, ratio)
		}
	}

	fs = buildFS(initial)
	collector = NewHostCollector(fs, WithCollectPerCoreCPU(false))
	if _, err := collector.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot disabled per-core first call returned error: %v", err)
	}
	fs["proc/stat"] = &fstest.MapFile{Data: []byte(second)}
	sample, err = collector.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot disabled per-core second call returned error: %v", err)
	}
	if len(sample.PerCoreUsageRatio) != 0 {
		t.Fatalf("expected no per-core ratios when collection disabled, got %d", len(sample.PerCoreUsageRatio))
	}
}
