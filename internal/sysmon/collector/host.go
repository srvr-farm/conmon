package collector

import (
	"io/fs"
	"os"
	"sync"
)

type HostSample struct {
	UptimeSeconds       float64
	BootID              string
	MemoryResidentBytes uint64
	TotalCPUUsageRatio  float64
	PerCoreUsageRatio   map[string]float64
}

type HostCollectorOption func(*HostCollector)

type HostCollector struct {
	fs             fs.FS
	collectPerCore bool

	mu   sync.Mutex
	prev map[string]cpuSnapshot
}

func NewHostCollector(fsys fs.FS, opts ...HostCollectorOption) *HostCollector {
	if fsys == nil {
		fsys = os.DirFS("/")
	}

	h := &HostCollector{
		fs:             fsys,
		collectPerCore: true,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func WithCollectPerCoreCPU(enabled bool) HostCollectorOption {
	return func(h *HostCollector) {
		h.collectPerCore = enabled
	}
}
