package metrics

import (
	"testing"

	"github.com/mcallan/conmon/internal/sysmon/collector"
	dto "github.com/prometheus/client_model/go"
)

func TestExporterEmitsHostAndServiceMetrics(t *testing.T) {
	exporter := NewExporter()
	exporter.UpdateHost("edge-a", collector.HostSample{
		UptimeSeconds:       120,
		BootID:              "boot-123",
		MemoryResidentBytes: 8192,
		TotalCPUUsageRatio:  0.25,
		PerCoreUsageRatio: map[string]float64{
			"cpu0": 0.20,
			"cpu1": 0.30,
		},
	})
	exporter.UpdateServices("edge-a", []ServiceSample{
		{
			Name:                 "docker.service",
			UptimeSeconds:        45,
			State:                "active",
			Active:               true,
			Enabled:              true,
			CPUUsageSecondsTotal: 3.5,
			MemoryResidentBytes:  2048,
		},
	})

	families := mustGather(t, exporter)
	if !containsMetricFamily(families, "sysmon_host_info") {
		t.Fatal("expected sysmon_host_info")
	}
	if findMetricWithLabels(families, "sysmon_host_info", map[string]string{"host": "edge-a", "boot_id": "boot-123"}) == nil {
		t.Fatal("expected host info with boot-123")
	}
	if findMetricWithLabels(families, "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) == nil {
		t.Fatal("expected sysmon_service_uptime_seconds for docker.service")
	}
	if findMetricWithLabels(families, "sysmon_service_state", map[string]string{"host": "edge-a", "service": "docker.service", "state": "active"}) == nil {
		t.Fatal("expected sysmon_service_state for docker.service/active")
	}
}

func TestExporterRemovesStaleServiceMetrics(t *testing.T) {
	exporter := NewExporter()
	exporter.UpdateServices("edge-a", []ServiceSample{{Name: "docker.service", State: "active", Active: true, Enabled: true}})
	if findMetricWithLabels(mustGather(t, exporter), "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) == nil {
		t.Fatal("initial docker.service uptime missing")
	}

	exporter.UpdateServices("edge-a", nil)
	families := mustGather(t, exporter)
	if findMetricWithLabels(families, "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) != nil {
		t.Fatal("stale docker.service metric still present")
	}
	if findMetricWithLabels(families, "sysmon_service_state", map[string]string{"host": "edge-a", "service": "docker.service"}) != nil {
		t.Fatal("stale docker.service state metric still present")
	}
}

func TestExporterServiceStateTransition(t *testing.T) {
	exporter := NewExporter()
	host, service := "edge-a", "docker.service"
	exporter.UpdateServices(host, []ServiceSample{{Name: service, State: "active"}})
	families := mustGather(t, exporter)
	requireMetricValue(t, families, "sysmon_service_state", map[string]string{"host": host, "service": service, "state": "active"}, 1)
	requireMetricValue(t, families, "sysmon_service_state", map[string]string{"host": host, "service": service, "state": "inactive"}, 0)

	exporter.UpdateServices(host, []ServiceSample{{Name: service, State: "inactive"}})
	families = mustGather(t, exporter)
	requireMetricValue(t, families, "sysmon_service_state", map[string]string{"host": host, "service": service, "state": "active"}, 0)
	requireMetricValue(t, families, "sysmon_service_state", map[string]string{"host": host, "service": service, "state": "inactive"}, 1)
}

func requireMetricValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string, want float64) {
	t.Helper()
	metric := findMetricWithLabels(families, name, labels)
	if metric == nil {
		t.Fatalf("missing metric %s labels %+v", name, labels)
	}
	gauge := metric.GetGauge()
	if gauge == nil {
		t.Fatalf("metric %s is not a gauge", name)
	}
	if got := gauge.GetValue(); got != want {
		t.Fatalf("metric %s labels %+v value = %v, want %v", name, labels, got, want)
	}
}

func TestExporterClearsStaleHostInfoBoots(t *testing.T) {
	exporter := NewExporter()
	exporter.UpdateHost("edge-a", collector.HostSample{BootID: "boot-1"})
	if findMetricWithLabels(mustGather(t, exporter), "sysmon_host_info", map[string]string{"host": "edge-a", "boot_id": "boot-1"}) == nil {
		t.Fatal("expected boot-1 host info")
	}

	exporter.UpdateHost("edge-a", collector.HostSample{BootID: "boot-2"})
	families := mustGather(t, exporter)
	if findMetricWithLabels(families, "sysmon_host_info", map[string]string{"host": "edge-a", "boot_id": "boot-1"}) != nil {
		t.Fatal("stale boot-1 host info still present")
	}
	if findMetricWithLabels(families, "sysmon_host_info", map[string]string{"host": "edge-a", "boot_id": "boot-2"}) == nil {
		t.Fatal("expected boot-2 host info")
	}
}

func mustGather(t *testing.T, exporter *Exporter) []*dto.MetricFamily {
	families, err := exporter.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}
	return families
}

func containsMetricFamily(families []*dto.MetricFamily, name string) bool {
	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}
	return false
}

func findMetricWithLabels(families []*dto.MetricFamily, familyName string, labels map[string]string) *dto.Metric {
	for _, family := range families {
		if family.GetName() != familyName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricHasLabels(metric, labels) {
				return metric
			}
		}
	}
	return nil
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for key, want := range labels {
		var found bool
		for _, label := range metric.GetLabel() {
			if label.GetName() == key && label.GetValue() == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestExporterPerCoreMetrics(t *testing.T) {
	// Ensure host per-core metrics register and observe entries.
	exporter := NewExporter()
	exporter.UpdateHost("edge-a", collector.HostSample{
		PerCoreUsageRatio: map[string]float64{"cpu0": 0.1},
	})

	if findMetricWithLabels(mustGather(t, exporter), "sysmon_host_cpu_core_usage_ratio", map[string]string{"host": "edge-a", "core": "cpu0"}) == nil {
		t.Fatal("expected per-core cpu usage metric")
	}
}
