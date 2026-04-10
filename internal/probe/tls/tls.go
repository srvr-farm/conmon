package tlsprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/result"
)

type Options struct {
	RootCAs *x509.CertPool
	Now     func() time.Time
}

type Probe struct {
	check  config.Check
	config *tls.Config
	now    func() time.Time
}

func New(check config.Check, options Options) *Probe {
	now := clockNow(options.Now)
	return &Probe{
		check:  check,
		config: buildTLSConfig(check, options.RootCAs, now),
		now:    now,
	}
}

func (p *Probe) Run(ctx context.Context) (outcome result.Result) {
	outcome = baseResult(p.check)
	start := time.Now()
	defer func() {
		outcome.Duration = time.Since(start)
	}()

	runCtx, cancel := context.WithTimeout(ctx, p.check.Timeout.Duration)
	defer cancel()

	address := net.JoinHostPort(p.check.Host, strconv.Itoa(p.check.Port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(runCtx, "tcp", address)
	if err != nil {
		return outcome
	}
	tlsConn := tls.Client(conn, p.config.Clone())
	err = tlsConn.HandshakeContext(runCtx)
	if err != nil {
		_ = conn.Close()
		return outcome
	}
	defer tlsConn.Close()

	peerCertificates := tlsConn.ConnectionState().PeerCertificates
	if len(peerCertificates) == 0 {
		return outcome
	}

	now := p.now()
	daysRemaining := int(math.Floor(peerCertificates[0].NotAfter.Sub(now).Hours() / 24))
	outcome.TLSCertDaysRemaining = daysRemaining
	outcome.Success = now.Before(peerCertificates[0].NotAfter) && daysRemaining >= p.check.MinDaysRemaining
	return outcome
}

func buildTLSConfig(check config.Check, rootCAs *x509.CertPool, now func() time.Time) *tls.Config {
	config := &tls.Config{
		RootCAs: rootCAs,
		Time:    now,
	}
	if check.ServerName != "" {
		config.ServerName = check.ServerName
	} else if check.Host != "" {
		config.ServerName = check.Host
	}
	return config
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

func clockNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}
