package icmpprobe

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func TestProbeRunSucceedsOnEchoReply(t *testing.T) {
	conn := &fakePacketConn{
		readPackets: [][]byte{mustMarshalMessage(t, &icmp.Message{
			Type: ipv4.ICMPTypeEchoReply,
			Code: 0,
			Body: &icmp.Echo{
				ID:   1234,
				Seq:  1,
				Data: []byte("conmon"),
			},
		})},
	}
	check := config.Check{
		ID:      "public-ping",
		Name:    "Public Ping",
		Group:   "internet",
		Kind:    "icmp",
		Scope:   "external",
		Host:    "8.8.8.8",
		Timeout: config.Duration{Duration: 250 * time.Millisecond},
	}

	result := New(check, Options{
		ID: 1234,
		ResolveIP: func(ctx context.Context, network, host string) (*net.IPAddr, error) {
			return &net.IPAddr{IP: net.ParseIP("8.8.8.8")}, nil
		},
		OpenPacketConn: func(ctx context.Context, network string) (packetConn, error) {
			if network != "ip4:icmp" {
				t.Fatalf("network = %q, want ip4:icmp", network)
			}
			return conn, nil
		},
	})

	outcome := result.Run(context.Background())

	if !outcome.Success {
		t.Fatal("expected success")
	}
	if got, want := outcome.ICMPEchoReplies, 1; got != want {
		t.Fatalf("ICMPEchoReplies = %d, want %d", got, want)
	}
	if len(conn.writes) != 1 {
		t.Fatalf("len(conn.writes) = %d, want 1", len(conn.writes))
	}
	if got, want := conn.writeTargets[0].String(), "8.8.8.8"; got != want {
		t.Fatalf("write target = %q, want %q", got, want)
	}
}

func TestProbeRunFailsWithoutEchoReplyBeforeDeadline(t *testing.T) {
	conn := &fakePacketConn{
		readErr: &net.OpError{Err: os.ErrDeadlineExceeded},
	}
	check := config.Check{
		ID:      "router-ping",
		Name:    "Router Ping",
		Group:   "internet",
		Kind:    "icmp",
		Scope:   "internal",
		Host:    "10.0.0.1",
		Count:   2,
		Timeout: config.Duration{Duration: 100 * time.Millisecond},
	}

	probe := New(check, Options{
		ID: 7,
		ResolveIP: func(ctx context.Context, network, host string) (*net.IPAddr, error) {
			return &net.IPAddr{IP: net.ParseIP("10.0.0.1")}, nil
		},
		OpenPacketConn: func(ctx context.Context, network string) (packetConn, error) {
			return conn, nil
		},
	})

	outcome := probe.Run(context.Background())

	if outcome.Success {
		t.Fatal("expected failure")
	}
	if got := outcome.ICMPEchoReplies; got != 0 {
		t.Fatalf("ICMPEchoReplies = %d, want 0", got)
	}
	if len(conn.writes) != 2 {
		t.Fatalf("len(conn.writes) = %d, want 2", len(conn.writes))
	}
	if conn.deadline.IsZero() {
		t.Fatal("expected deadline to be set")
	}
}

func TestProbeRunCountsMatchingRepliesUpToConfiguredCount(t *testing.T) {
	conn := &fakePacketConn{
		readPackets: [][]byte{
			mustMarshalMessage(t, &icmp.Message{
				Type: ipv4.ICMPTypeEchoReply,
				Code: 0,
				Body: &icmp.Echo{ID: 55, Seq: 1, Data: []byte("conmon")},
			}),
			mustMarshalMessage(t, &icmp.Message{
				Type: ipv4.ICMPTypeEchoReply,
				Code: 0,
				Body: &icmp.Echo{ID: 55, Seq: 2, Data: []byte("conmon")},
			}),
			mustMarshalMessage(t, &icmp.Message{
				Type: ipv4.ICMPTypeEchoReply,
				Code: 0,
				Body: &icmp.Echo{ID: 55, Seq: 3, Data: []byte("conmon")},
			}),
		},
		readErr: &net.OpError{Err: os.ErrDeadlineExceeded},
	}
	check := config.Check{
		ID:      "router-ping",
		Name:    "Router Ping",
		Group:   "internet",
		Kind:    "icmp",
		Scope:   "internal",
		Host:    "10.0.0.1",
		Count:   3,
		Timeout: config.Duration{Duration: 100 * time.Millisecond},
	}

	probe := New(check, Options{
		ID: 55,
		ResolveIP: func(ctx context.Context, network, host string) (*net.IPAddr, error) {
			return &net.IPAddr{IP: net.ParseIP("10.0.0.1")}, nil
		},
		OpenPacketConn: func(ctx context.Context, network string) (packetConn, error) {
			return conn, nil
		},
	})

	outcome := probe.Run(context.Background())

	if !outcome.Success {
		t.Fatal("expected success")
	}
	if got, want := outcome.ICMPEchoReplies, 3; got != want {
		t.Fatalf("ICMPEchoReplies = %d, want %d", got, want)
	}
	if len(conn.writes) != 3 {
		t.Fatalf("len(conn.writes) = %d, want 3", len(conn.writes))
	}
}

type fakePacketConn struct {
	deadline     time.Time
	writes       [][]byte
	writeTargets []net.Addr
	readPackets  [][]byte
	readErr      error
}

func (f *fakePacketConn) SetDeadline(deadline time.Time) error {
	f.deadline = deadline
	return nil
}

func (f *fakePacketConn) WriteTo(payload []byte, address net.Addr) (int, error) {
	f.writes = append(f.writes, append([]byte(nil), payload...))
	f.writeTargets = append(f.writeTargets, address)
	return len(payload), nil
}

func (f *fakePacketConn) ReadFrom(payload []byte) (int, net.Addr, error) {
	if len(f.readPackets) == 0 {
		return 0, &net.IPAddr{IP: net.ParseIP("127.0.0.1")}, f.readErr
	}
	packet := f.readPackets[0]
	f.readPackets = f.readPackets[1:]
	copy(payload, packet)
	return len(packet), &net.IPAddr{IP: net.ParseIP("8.8.8.8")}, nil
}

func (f *fakePacketConn) Close() error {
	return nil
}

func mustMarshalMessage(t *testing.T, message *icmp.Message) []byte {
	t.Helper()

	payload, err := message.Marshal(nil)
	if err != nil {
		t.Fatalf("message.Marshal returned error: %v", err)
	}
	return payload
}
