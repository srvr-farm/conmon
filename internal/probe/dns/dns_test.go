package dnsprobe

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/config"
	mdns "github.com/miekg/dns"
)

func TestProbeRunSucceedsOnNoErrorAnswer(t *testing.T) {
	server, address := startDNSServer(t, func(message *mdns.Msg) *mdns.Msg {
		response := new(mdns.Msg)
		response.SetReply(message)
		response.Answer = append(response.Answer, &mdns.A{
			Hdr: mdns.RR_Header{
				Name:   message.Question[0].Name,
				Rrtype: mdns.TypeA,
				Class:  mdns.ClassINET,
				Ttl:    30,
			},
			A: net.ParseIP("192.0.2.10").To4(),
		})
		return response
	})
	defer server.Shutdown()

	check := config.Check{
		ID:         "dns",
		Name:       "Resolver",
		Group:      "internet",
		Kind:       "dns",
		Scope:      "internal",
		Server:     "127.0.0.1",
		Port:       portFromAddress(t, address),
		QueryName:  "callanarchitects.com",
		RecordType: "A",
		Timeout:    config.Duration{Duration: time.Second},
	}

	result := New(check, Options{}).Run(context.Background())

	if !result.Success {
		t.Fatal("expected success")
	}
	if got, want := result.DNSRCode, mdns.RcodeSuccess; got != want {
		t.Fatalf("DNSRCode = %d, want %d", got, want)
	}
	if got, want := result.DNSAnswerCount, 1; got != want {
		t.Fatalf("DNSAnswerCount = %d, want %d", got, want)
	}
}

func TestProbeRunSucceedsWhenExpectedRCodeMatches(t *testing.T) {
	server, address := startDNSServer(t, func(message *mdns.Msg) *mdns.Msg {
		response := new(mdns.Msg)
		response.SetReply(message)
		response.Rcode = mdns.RcodeNameError
		return response
	})
	defer server.Shutdown()

	check := config.Check{
		ID:                 "dns",
		Name:               "Resolver",
		Group:              "internet",
		Kind:               "dns",
		Scope:              "internal",
		Server:             "127.0.0.1",
		Port:               portFromAddress(t, address),
		QueryName:          "missing.callanarchitects.com",
		RecordType:         "A",
		ExpectedRCode:      "NXDOMAIN",
		ExpectedRCodeValue: mdns.RcodeNameError,
		Timeout:            config.Duration{Duration: time.Second},
	}

	result := New(check, Options{}).Run(context.Background())

	if !result.Success {
		t.Fatal("expected success")
	}
	if got, want := result.DNSRCode, mdns.RcodeNameError; got != want {
		t.Fatalf("DNSRCode = %d, want %d", got, want)
	}
	if got := result.DNSAnswerCount; got != 0 {
		t.Fatalf("DNSAnswerCount = %d, want 0", got)
	}
}

func TestProbeRunFailsOnNoErrorWithoutAnswers(t *testing.T) {
	server, address := startDNSServer(t, func(message *mdns.Msg) *mdns.Msg {
		response := new(mdns.Msg)
		response.SetReply(message)
		return response
	})
	defer server.Shutdown()

	check := config.Check{
		ID:         "dns",
		Name:       "Resolver",
		Group:      "internet",
		Kind:       "dns",
		Scope:      "internal",
		Server:     "127.0.0.1",
		Port:       portFromAddress(t, address),
		QueryName:  "callanarchitects.com",
		RecordType: "A",
		Timeout:    config.Duration{Duration: time.Second},
	}

	result := New(check, Options{}).Run(context.Background())

	if result.Success {
		t.Fatal("expected failure")
	}
	if got, want := result.DNSRCode, mdns.RcodeSuccess; got != want {
		t.Fatalf("DNSRCode = %d, want %d", got, want)
	}
	if got := result.DNSAnswerCount; got != 0 {
		t.Fatalf("DNSAnswerCount = %d, want 0", got)
	}
}

func TestProbeRunFailsOnUnknownRecordType(t *testing.T) {
	called := false
	check := config.Check{
		ID:         "dns",
		Name:       "Resolver",
		Group:      "internet",
		Kind:       "dns",
		Scope:      "internal",
		Server:     "127.0.0.1",
		Port:       53,
		QueryName:  "callanarchitects.com",
		RecordType: "BOGUS",
		Timeout:    config.Duration{Duration: time.Second},
	}

	result := New(check, Options{
		Exchange: func(ctx context.Context, client *mdns.Client, message *mdns.Msg, address string) (*mdns.Msg, time.Duration, error) {
			called = true
			return nil, 0, nil
		},
	}).Run(context.Background())

	if result.Success {
		t.Fatal("expected failure")
	}
	if called {
		t.Fatal("unexpected dns exchange for unknown record type")
	}
}

func startDNSServer(t *testing.T, handler func(*mdns.Msg) *mdns.Msg) (*mdns.Server, string) {
	t.Helper()

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.ListenPacket returned error: %v", err)
	}
	server := &mdns.Server{
		PacketConn: packetConn,
		Handler: mdns.HandlerFunc(func(w mdns.ResponseWriter, request *mdns.Msg) {
			response := handler(request)
			if err := w.WriteMsg(response); err != nil {
				t.Fatalf("WriteMsg returned error: %v", err)
			}
		}),
	}
	go func() {
		if err := server.ActivateAndServe(); err != nil {
			t.Errorf("ActivateAndServe returned error: %v", err)
		}
	}()
	return server, packetConn.LocalAddr().String()
}

func portFromAddress(t *testing.T, address string) int {
	t.Helper()

	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("net.SplitHostPort returned error: %v", err)
	}
	port, err := net.LookupPort("udp", portText)
	if err != nil {
		t.Fatalf("net.LookupPort returned error: %v", err)
	}
	return port
}
