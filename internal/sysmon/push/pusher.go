package push

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
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

// Push sends registry contents to the configured Pushgateway with host grouping.
func (p *Pusher) Push(ctx context.Context, host string, reg prometheus.Gatherer) error {
	jobPusher := push.New(p.baseURL, p.job).Client(p.client).Gatherer(reg).Grouping("host", host)
	if err := jobPusher.PushContext(ctx); err != nil {
		return fmt.Errorf("pushgateway push job=%s host=%s: %w", p.job, host, err)
	}
	return nil
}
