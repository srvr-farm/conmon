package push

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	dto "github.com/prometheus/client_model/go"
)

// Pusher pushes sysmon metrics to a Pushgateway.
type Pusher struct {
	baseURL string
	job     string
	client  *http.Client
}

// New returns a new Pushgateway Pusher.
func New(baseURL, job string) *Pusher {
	return &Pusher{
		baseURL: baseURL,
		job:     job,
		client:  http.DefaultClient,
	}
}

type strippingGatherer struct {
	g         prometheus.Gatherer
	labelName string
}

func (sg strippingGatherer) Gather() ([]*dto.MetricFamily, error) {
	mfs, err := sg.g.Gather()
	if err != nil {
		return nil, err
	}

	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			if m == nil || len(m.Label) == 0 {
				continue
			}
			dst := m.Label[:0]
			for _, lp := range m.Label {
				if lp == nil || lp.GetName() != sg.labelName {
					dst = append(dst, lp)
				}
			}
			m.Label = dst
		}
	}

	return mfs, nil
}

// Push sends registry contents to the configured Pushgateway with host grouping.
func (p *Pusher) Push(ctx context.Context, host string, reg prometheus.Gatherer) error {
	jobPusher := push.New(p.baseURL, p.job).
		Client(p.client).
		Gatherer(strippingGatherer{g: reg, labelName: "host"}).
		Grouping("host", host)
	if err := jobPusher.PushContext(ctx); err != nil {
		return fmt.Errorf("pushgateway push job=%s host=%s: %w", p.job, host, err)
	}
	return nil
}
