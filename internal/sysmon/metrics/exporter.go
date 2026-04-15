package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/mcallan/conmon/internal/sysmon/collector"
)

// ServiceSample captures the state that sysmon exposes for a service on a host.
type ServiceSample struct {
	Name                 string
	UptimeSeconds        float64
	State                string
	Active               bool
	Enabled              bool
	CPUUsageSecondsTotal float64
	MemoryResidentBytes  uint64
}

// Exporter exposes sysmon host and service metrics via Prometheus families.
type Exporter struct {
	registry *prometheus.Registry

	hostCPUUsage   *prometheus.GaugeVec
	hostCoreUsage  *prometheus.GaugeVec
	hostMemory     *prometheus.GaugeVec
	hostUptime     *prometheus.GaugeVec
	hostInfo       *prometheus.GaugeVec
	serviceUptime  *prometheus.GaugeVec
	serviceState   *prometheus.GaugeVec
	serviceActive  *prometheus.GaugeVec
	serviceEnabled *prometheus.GaugeVec
	serviceCPU     *prometheus.GaugeVec
	serviceMemory  *prometheus.GaugeVec

	mu             sync.Mutex
	servicesByHost map[string]map[string]struct{}
	coresByHost    map[string]map[string]struct{}
}

// NewExporter returns a new exporter with all sysmon metric families registered.
func NewExporter() *Exporter {
	exporter := &Exporter{
		registry:       prometheus.NewRegistry(),
		servicesByHost: make(map[string]map[string]struct{}),
		coresByHost:    make(map[string]map[string]struct{}),
	}

	exporter.hostCPUUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_host_cpu_usage_ratio",
		Help: "Total CPU usage ratio reported for a host",
	}, []string{"host"})
	exporter.hostCoreUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_host_cpu_core_usage_ratio",
		Help: "Per-core CPU usage ratio for a host",
	}, []string{"host", "core"})
	exporter.hostMemory = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_host_memory_resident_bytes",
		Help: "Resident memory used by the host",
	}, []string{"host"})
	exporter.hostUptime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_host_uptime_seconds",
		Help: "Uptime in seconds for the host",
	}, []string{"host"})
	exporter.hostInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_host_info",
		Help: "Host metadata for sysmon",
	}, []string{"host", "boot_id"})
	exporter.serviceUptime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_uptime_seconds",
		Help: "Uptime for a host service",
	}, []string{"host", "service"})
	exporter.serviceState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_state",
		Help: "Service state labels for a host service",
	}, []string{"host", "service", "state"})
	exporter.serviceActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_active",
		Help: "Whether a service is active",
	}, []string{"host", "service"})
	exporter.serviceEnabled = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_enabled",
		Help: "Whether a service is enabled",
	}, []string{"host", "service"})
	exporter.serviceCPU = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_cpu_usage_seconds_total",
		Help: "CPU usage seconds for a service",
	}, []string{"host", "service"})
	exporter.serviceMemory = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sysmon_service_memory_resident_bytes",
		Help: "Resident memory used by a service",
	}, []string{"host", "service"})

	exporter.registry.MustRegister(
		exporter.hostCPUUsage,
		exporter.hostCoreUsage,
		exporter.hostMemory,
		exporter.hostUptime,
		exporter.hostInfo,
		exporter.serviceUptime,
		exporter.serviceState,
		exporter.serviceActive,
		exporter.serviceEnabled,
		exporter.serviceCPU,
		exporter.serviceMemory,
	)

	return exporter
}

// UpdateHost records the latest host and service data.
func (e *Exporter) UpdateHost(host string, hostSample collector.HostSample, services []ServiceSample) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.recordHost(host, hostSample)
	serviceSet := make(map[string]struct{})
	for _, svc := range services {
		if svc.Name == "" {
			continue
		}
		serviceSet[svc.Name] = struct{}{}
		e.recordService(host, svc)
	}

	if prev, ok := e.servicesByHost[host]; ok {
		for service := range prev {
			if _, keep := serviceSet[service]; !keep {
				e.clearServiceSeries(host, service)
			}
		}
	}
	e.servicesByHost[host] = serviceSet
}

func (e *Exporter) recordHost(host string, sample collector.HostSample) {
	labels := prometheus.Labels{"host": host}
	e.hostUptime.With(labels).Set(sample.UptimeSeconds)
	e.hostMemory.With(labels).Set(float64(sample.MemoryResidentBytes))
	e.hostCPUUsage.With(labels).Set(sample.TotalCPUUsageRatio)
	if sample.BootID != "" {
		e.hostInfo.With(prometheus.Labels{"host": host, "boot_id": sample.BootID}).Set(1)
	} else {
		e.hostInfo.With(prometheus.Labels{"host": host, "boot_id": ""}).Set(1)
	}

	coreSet := make(map[string]struct{})
	for core, usage := range sample.PerCoreUsageRatio {
		coreLabels := prometheus.Labels{"host": host, "core": core}
		e.hostCoreUsage.With(coreLabels).Set(usage)
		coreSet[core] = struct{}{}
	}
	if prev, ok := e.coresByHost[host]; ok {
		for core := range prev {
			if _, keep := coreSet[core]; !keep {
				e.hostCoreUsage.Delete(prometheus.Labels{"host": host, "core": core})
			}
		}
	}
	e.coresByHost[host] = coreSet
}

func (e *Exporter) recordService(host string, svc ServiceSample) {
	labels := prometheus.Labels{"host": host, "service": svc.Name}
	e.serviceUptime.With(labels).Set(svc.UptimeSeconds)
	e.serviceActive.With(labels).Set(boolToFloat64(svc.Active))
	e.serviceEnabled.With(labels).Set(boolToFloat64(svc.Enabled))
	e.serviceCPU.With(labels).Set(svc.CPUUsageSecondsTotal)
	e.serviceMemory.With(labels).Set(float64(svc.MemoryResidentBytes))
	e.serviceState.DeletePartialMatch(prometheus.Labels{"host": host, "service": svc.Name})
	if svc.State != "" {
		stateLabels := prometheus.Labels{"host": host, "service": svc.Name, "state": svc.State}
		e.serviceState.With(stateLabels).Set(1)
	}
}

func (e *Exporter) clearServiceSeries(host, service string) {
	labels := prometheus.Labels{"host": host, "service": service}
	e.serviceUptime.Delete(labels)
	e.serviceActive.Delete(labels)
	e.serviceEnabled.Delete(labels)
	e.serviceCPU.Delete(labels)
	e.serviceMemory.Delete(labels)
	e.serviceState.DeletePartialMatch(labels)
}

func boolToFloat64(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// Gather returns the latest metric families from the exporter registry.
func (e *Exporter) Gather() ([]*dto.MetricFamily, error) {
    return e.registry.Gather()
}
