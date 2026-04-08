package metrics

import (
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/result"
	dto "github.com/prometheus/client_model/go"
)

func TestExporterRecordsExpectedMetricFamilies(t *testing.T) {
	exporter, err := New([]string{"site"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	exporter.Record(result.Result{
		CheckID:        "web",
		CheckName:      "Public Web",
		CheckGroup:     "internet",
		CheckKind:      "https",
		CheckScope:     "external",
		Labels:         map[string]string{"site": "edge"},
		Success:        true,
		Duration:       250 * time.Millisecond,
		HTTPStatusCode: 204,
	})
	exporter.Record(result.Result{
		CheckID:              "cert",
		CheckName:            "Example Certificate",
		CheckGroup:           "external-services",
		CheckKind:            "tls",
		CheckScope:           "external",
		Labels:               map[string]string{"site": "edge"},
		Success:              false,
		Duration:             500 * time.Millisecond,
		TLSCertDaysRemaining: 3,
	})

	families, err := exporter.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	assertMetricFamilyNames(t, families,
		"conmon_probe_success",
		"conmon_probe_duration_seconds",
		"conmon_probe_runs_total",
		"conmon_http_status_code",
		"conmon_tls_cert_days_remaining",
	)
	assertGaugeValue(t, families, "conmon_http_status_code", 204, "web")
	assertGaugeValue(t, families, "conmon_tls_cert_days_remaining", 3, "cert")
	assertCounterValue(t, families, "conmon_probe_runs_total", 1, "web")
	assertCounterValue(t, families, "conmon_probe_runs_total", 1, "cert")
	assertMetricMissing(t, families, "conmon_http_status_code", "cert")
	assertMetricMissing(t, families, "conmon_tls_cert_days_remaining", "web")
}

func TestExporterRecordsDNSRCodeOnlyForDNSResults(t *testing.T) {
	exporter, err := New(nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	exporter.Record(result.Result{
		CheckID:        "dns",
		CheckName:      "Resolver",
		CheckGroup:     "internet",
		CheckKind:      "dns",
		CheckScope:     "internal",
		Success:        true,
		Duration:       100 * time.Millisecond,
		DNSRCode:       3,
		DNSAnswerCount: 0,
	})
	exporter.Record(result.Result{
		CheckID:        "web",
		CheckName:      "Web",
		CheckGroup:     "internet",
		CheckKind:      "https",
		CheckScope:     "external",
		Success:        true,
		Duration:       100 * time.Millisecond,
		HTTPStatusCode: 200,
	})

	families, err := exporter.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	assertMetricFamilyNames(t, families, "conmon_dns_rcode")
	assertGaugeValue(t, families, "conmon_dns_rcode", 3, "dns")
	assertMetricMissing(t, families, "conmon_dns_rcode", "web")
}

func TestNewRejectsInvalidCustomLabelKey(t *testing.T) {
	_, err := New([]string{"invalid-key"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsCustomLabelKeyThatCollidesWithBaseLabel(t *testing.T) {
	_, err := New([]string{"check_id"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func assertMetricFamilyNames(t *testing.T, families []*dto.MetricFamily, names ...string) {
	t.Helper()
	for _, name := range names {
		if findMetricFamily(families, name) == nil {
			t.Fatalf("metric family %q not found", name)
		}
	}
}

func assertGaugeValue(t *testing.T, families []*dto.MetricFamily, name string, want float64, checkID string) {
	t.Helper()
	family := findMetricFamily(families, name)
	if family == nil {
		t.Fatalf("metric family %q not found", name)
	}
	for _, metric := range family.Metric {
		for _, label := range metric.Label {
			if label.GetName() == "check_id" && label.GetValue() == checkID {
				got := metric.Gauge.GetValue()
				if got != want {
					t.Fatalf("%s[%s] = %v, want %v", name, checkID, got, want)
				}
				return
			}
		}
	}
	t.Fatalf("metric %q for check_id=%q not found", name, checkID)
}

func assertCounterValue(t *testing.T, families []*dto.MetricFamily, name string, want float64, checkID string) {
	t.Helper()
	family := findMetricFamily(families, name)
	if family == nil {
		t.Fatalf("metric family %q not found", name)
	}
	for _, metric := range family.Metric {
		for _, label := range metric.Label {
			if label.GetName() == "check_id" && label.GetValue() == checkID {
				got := metric.Counter.GetValue()
				if got != want {
					t.Fatalf("%s[%s] = %v, want %v", name, checkID, got, want)
				}
				return
			}
		}
	}
	t.Fatalf("metric %q for check_id=%q not found", name, checkID)
}

func assertMetricMissing(t *testing.T, families []*dto.MetricFamily, name string, checkID string) {
	t.Helper()
	family := findMetricFamily(families, name)
	if family == nil {
		t.Fatalf("metric family %q not found", name)
	}
	for _, metric := range family.Metric {
		for _, label := range metric.Label {
			if label.GetName() == "check_id" && label.GetValue() == checkID {
				t.Fatalf("metric %q for check_id=%q was unexpectedly present", name, checkID)
			}
		}
	}
}

func findMetricFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}
