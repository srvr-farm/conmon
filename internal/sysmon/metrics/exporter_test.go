package metrics

import (
    "testing"

    dto "github.com/prometheus/client_model/go"
    "github.com/mcallan/conmon/internal/sysmon/collector"
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
    }, []ServiceSample{
        {
            Name:                  "docker.service",
            UptimeSeconds:         45,
            State:                 "active",
            Active:                true,
            Enabled:               true,
            CPUUsageSecondsTotal:  3.5,
            MemoryResidentBytes:   2048,
        },
    })

    families, err := exporter.Gather()
    if err != nil {
        t.Fatalf("Gather returned error: %v", err)
    }
    if !containsMetricFamily(families, "sysmon_host_info") {
        t.Fatal("expected sysmon_host_info")
    }
    if findMetricWithLabels(families, "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) == nil {
        t.Fatal("expected sysmon_service_uptime_seconds for docker.service")
    }
}

func TestExporterRemovesStaleServiceMetrics(t *testing.T) {
    exporter := NewExporter()
    exporter.UpdateHost("edge-a", collector.HostSample{}, []ServiceSample{
        {Name: "docker.service", State: "active", Active: true, Enabled: true},
    })
    if findMetricWithLabels(mustGather(t, exporter), "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) == nil {
        t.Fatal("initial docker.service uptime missing")
    }

    exporter.UpdateHost("edge-a", collector.HostSample{}, nil)
    families := mustGather(t, exporter)
    if findMetricWithLabels(families, "sysmon_service_uptime_seconds", map[string]string{"host": "edge-a", "service": "docker.service"}) != nil {
        t.Fatal("stale docker.service metric still present")
    }
    if findMetricWithLabels(families, "sysmon_service_state", map[string]string{"host": "edge-a", "service": "docker.service"}) != nil {
        t.Fatal("stale docker.service state metric still present")
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
    }, nil)

    if findMetricWithLabels(mustGather(t, exporter), "sysmon_host_cpu_core_usage_ratio", map[string]string{"host": "edge-a", "core": "cpu0"}) == nil {
        t.Fatal("expected per-core cpu usage metric")
    }
}
