package dnsprobe

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/probe"
	"github.com/mcallan/conmon/internal/result"
	mdns "github.com/miekg/dns"
)

type exchangeFunc func(ctx context.Context, client *mdns.Client, message *mdns.Msg, address string) (*mdns.Msg, time.Duration, error)

type Options struct {
	Client   *mdns.Client
	Exchange exchangeFunc
}

type Probe struct {
	check    config.Check
	client   *mdns.Client
	exchange exchangeFunc
}

func New(check config.Check, options Options) *Probe {
	client := options.Client
	if client == nil {
		client = &mdns.Client{
			Net:     "udp",
			Timeout: check.Timeout.Duration,
		}
	}
	return &Probe{
		check:    check,
		client:   client,
		exchange: dnsExchange(options.Exchange),
	}
}

func (p *Probe) Run(ctx context.Context) (outcome result.Result) {
	outcome = probe.BaseResult(p.check)
	start := time.Now()
	defer func() {
		outcome.Duration = time.Since(start)
	}()

	runCtx, cancel := context.WithTimeout(ctx, p.check.Timeout.Duration)
	defer cancel()

	questionType, ok := recordType(p.check.RecordType)
	if !ok {
		return outcome
	}

	message := new(mdns.Msg)
	message.SetQuestion(mdns.Fqdn(p.check.QueryName), questionType)

	response, _, err := p.exchange(runCtx, p.client, message, net.JoinHostPort(p.check.Server, strconv.Itoa(p.check.Port)))
	if err != nil || response == nil {
		return outcome
	}

	outcome.DNSRCode = response.Rcode
	outcome.DNSAnswerCount = len(response.Answer)
	if p.check.ExpectedRCode != "" {
		outcome.Success = response.Rcode == p.check.ExpectedRCodeValue
		return outcome
	}

	outcome.Success = response.Rcode == mdns.RcodeSuccess && len(response.Answer) > 0
	return outcome
}

func dnsExchange(exchange exchangeFunc) exchangeFunc {
	if exchange != nil {
		return exchange
	}
	return func(ctx context.Context, client *mdns.Client, message *mdns.Msg, address string) (*mdns.Msg, time.Duration, error) {
		return client.ExchangeContext(ctx, message, address)
	}
}

func recordType(recordType string) (uint16, bool) {
	switch recordType {
	case "AAAA":
		return mdns.TypeAAAA, true
	case "CNAME":
		return mdns.TypeCNAME, true
	case "MX":
		return mdns.TypeMX, true
	case "NS":
		return mdns.TypeNS, true
	case "PTR":
		return mdns.TypePTR, true
	case "SOA":
		return mdns.TypeSOA, true
	case "SRV":
		return mdns.TypeSRV, true
	case "TXT":
		return mdns.TypeTXT, true
	case "A":
		return mdns.TypeA, true
	default:
		return 0, false
	}
}
