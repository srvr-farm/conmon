package metrics

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/mcallan/conmon/internal/result"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

var baseLabels = []string{
	"check_id",
	"check_name",
	"group",
	"kind",
	"scope",
}

var labelKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Exporter struct {
	registry      *prometheus.Registry
	labelKeys     []string
	probeSuccess  *prometheus.GaugeVec
	probeDuration *prometheus.GaugeVec
	probeRuns     *prometheus.CounterVec
	httpStatus    *prometheus.GaugeVec
	dnsRCode      *prometheus.GaugeVec
	tlsDays       *prometheus.GaugeVec
}

func New(customLabelKeys []string) (*Exporter, error) {
	labelKeys, err := normalizedLabelKeys(customLabelKeys)
	if err != nil {
		return nil, err
	}
	labelKeys = append(append([]string(nil), baseLabels...), labelKeys...)
	exporter := &Exporter{
		registry:  prometheus.NewRegistry(),
		labelKeys: labelKeys,
		probeSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "conmon_probe_success",
			Help: "Latest probe success result.",
		}, labelKeys),
		probeDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "conmon_probe_duration_seconds",
			Help: "Latest probe duration in seconds.",
		}, labelKeys),
		probeRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "conmon_probe_runs_total",
			Help: "Total number of probe runs.",
		}, labelKeys),
		httpStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "conmon_http_status_code",
			Help: "Latest observed HTTP status code.",
		}, labelKeys),
		dnsRCode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "conmon_dns_rcode",
			Help: "Latest observed DNS response code.",
		}, labelKeys),
		tlsDays: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "conmon_tls_cert_days_remaining",
			Help: "Latest observed whole TLS certificate days remaining.",
		}, labelKeys),
	}
	exporter.registry.MustRegister(
		exporter.probeSuccess,
		exporter.probeDuration,
		exporter.probeRuns,
		exporter.httpStatus,
		exporter.dnsRCode,
		exporter.tlsDays,
	)
	return exporter, nil
}

func (e *Exporter) Record(outcome result.Result) {
	labels := e.labelValues(outcome)
	e.probeSuccess.WithLabelValues(labels...).Set(boolFloat64(outcome.Success))
	e.probeDuration.WithLabelValues(labels...).Set(outcome.Duration.Seconds())
	e.probeRuns.WithLabelValues(labels...).Inc()
	switch outcome.CheckKind {
	case "https":
		e.httpStatus.WithLabelValues(labels...).Set(float64(outcome.HTTPStatusCode))
	case "dns":
		e.dnsRCode.WithLabelValues(labels...).Set(float64(outcome.DNSRCode))
	case "tls":
		e.tlsDays.WithLabelValues(labels...).Set(float64(outcome.TLSCertDaysRemaining))
	}
}

func (e *Exporter) Handler() http.Handler {
	return promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{})
}

func (e *Exporter) Gather() ([]*dto.MetricFamily, error) {
	return e.registry.Gather()
}

func (e *Exporter) labelValues(outcome result.Result) []string {
	values := make([]string, 0, len(e.labelKeys))
	values = append(values,
		outcome.CheckID,
		outcome.CheckName,
		outcome.CheckGroup,
		outcome.CheckKind,
		outcome.CheckScope,
	)
	for _, key := range e.labelKeys[len(baseLabels):] {
		values = append(values, outcome.Labels[key])
	}
	return values
}

func normalizedLabelKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	baseLabelSet := make(map[string]struct{}, len(baseLabels))
	for _, key := range baseLabels {
		baseLabelSet[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return nil, fmt.Errorf("custom label key is required")
		}
		if strings.HasPrefix(trimmed, "__") {
			return nil, fmt.Errorf("custom label key %q is reserved", trimmed)
		}
		if !labelKeyPattern.MatchString(trimmed) {
			return nil, fmt.Errorf("custom label key %q is invalid", trimmed)
		}
		if _, ok := baseLabelSet[trimmed]; ok {
			return nil, fmt.Errorf("custom label key %q collides with built-in label", trimmed)
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	slices.Sort(normalized)
	return normalized, nil
}

func boolFloat64(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
