package httpprobe

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/result"
)

type Options struct {
	Client *http.Client
}

type Probe struct {
	check          config.Check
	client         *http.Client
	expectedStatus []int
}

func New(check config.Check, options Options) *Probe {
	return &Probe{
		check:          check,
		client:         buildClient(check, options.Client),
		expectedStatus: expectedStatuses(check.ExpectedStatus),
	}
}

func (p *Probe) Run(ctx context.Context) (outcome result.Result) {
	outcome = baseResult(p.check)
	start := time.Now()
	defer func() {
		outcome.Duration = time.Since(start)
	}()

	request, err := http.NewRequestWithContext(ctx, p.check.Method, p.check.URL, nil)
	if err != nil {
		return outcome
	}
	for key, value := range p.check.Headers {
		request.Header.Set(key, value)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return outcome
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	outcome.HTTPStatusCode = response.StatusCode
	outcome.Success = slices.Contains(p.expectedStatus, response.StatusCode)
	return outcome
}

func buildClient(check config.Check, client *http.Client) *http.Client {
	if client != nil {
		return client
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if check.TLSServerName != "" {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.ServerName = check.TLSServerName
	}

	return &http.Client{
		Timeout:   check.Timeout.Duration,
		Transport: transport,
	}
}

func baseResult(check config.Check) result.Result {
	return result.Result{
		CheckID:    check.ID,
		CheckName:  check.Name,
		CheckGroup: check.Group,
		CheckKind:  check.Kind,
		CheckScope: check.Scope,
		Labels:     cloneLabels(check.Labels),
	}
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func expectedStatuses(statuses []int) []int {
	if len(statuses) == 0 {
		return []int{http.StatusOK}
	}
	return append([]int(nil), statuses...)
}
