package icmpprobe

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/mcallan/conmon/internal/config"
	"github.com/mcallan/conmon/internal/probe"
	"github.com/mcallan/conmon/internal/result"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type packetConn interface {
	SetDeadline(deadline time.Time) error
	WriteTo(payload []byte, address net.Addr) (int, error)
	ReadFrom(payload []byte) (int, net.Addr, error)
	Close() error
}

type resolveIPFunc func(ctx context.Context, network, host string) (*net.IPAddr, error)
type openPacketConnFunc func(ctx context.Context, network string) (packetConn, error)

type Options struct {
	ID             int
	ResolveIP      resolveIPFunc
	OpenPacketConn openPacketConnFunc
}

type Probe struct {
	check          config.Check
	id             int
	resolveIP      resolveIPFunc
	openPacketConn openPacketConnFunc
}

func New(check config.Check, options Options) *Probe {
	return &Probe{
		check:          check,
		id:             echoID(options.ID),
		resolveIP:      resolveIP(options.ResolveIP),
		openPacketConn: packetOpener(options.OpenPacketConn),
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

	target, err := p.resolveIP(runCtx, "ip", p.check.Host)
	if err != nil || target == nil {
		return outcome
	}

	network, protocol, requestType, replyType := protocolForIP(target.IP)
	conn, err := p.openPacketConn(runCtx, network)
	if err != nil {
		return outcome
	}
	defer conn.Close()

	if deadline, ok := runCtx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return outcome
		}
	}

	sent := 0
	for sequence := 1; sequence <= probeCount(p.check.Count); sequence++ {
		if runCtx.Err() != nil {
			break
		}
		payload, err := (&icmp.Message{
			Type: requestType,
			Code: 0,
			Body: &icmp.Echo{
				ID:   p.id,
				Seq:  sequence,
				Data: []byte("conmon"),
			},
		}).Marshal(nil)
		if err != nil {
			return outcome
		}
		if _, err := conn.WriteTo(payload, target); err != nil {
			return outcome
		}
		sent++
	}
	if sent == 0 {
		return outcome
	}

	buffer := make([]byte, 1500)
	seenReplies := make(map[int]struct{}, sent)
	for {
		length, _, err := conn.ReadFrom(buffer)
		if err != nil {
			outcome.Success = outcome.ICMPEchoReplies > 0
			return outcome
		}

		message, err := icmp.ParseMessage(protocol, buffer[:length])
		if err != nil {
			continue
		}
		if message.Type != replyType {
			continue
		}

		body, ok := message.Body.(*icmp.Echo)
		if !ok || body.ID != p.id || body.Seq < 1 || body.Seq > sent {
			continue
		}
		if _, seen := seenReplies[body.Seq]; seen {
			continue
		}

		seenReplies[body.Seq] = struct{}{}
		outcome.ICMPEchoReplies++
		outcome.Success = true
		if outcome.ICMPEchoReplies >= sent {
			return outcome
		}
	}
}

func echoID(id int) int {
	if id != 0 {
		return id
	}
	return int(uint16(time.Now().UnixNano()))
}

func resolveIP(resolve resolveIPFunc) resolveIPFunc {
	if resolve != nil {
		return resolve
	}
	return func(ctx context.Context, network, host string) (*net.IPAddr, error) {
		if parsed := net.ParseIP(host); parsed != nil {
			return &net.IPAddr{IP: parsed}, nil
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(addresses) == 0 {
			return nil, errors.New("no ip addresses resolved")
		}
		return &addresses[0], nil
	}
}

func packetOpener(open openPacketConnFunc) openPacketConnFunc {
	if open != nil {
		return open
	}
	return func(ctx context.Context, network string) (packetConn, error) {
		return icmp.ListenPacket(network, "")
	}
}

func protocolForIP(ip net.IP) (network string, protocol int, requestType icmp.Type, replyType icmp.Type) {
	if ip.To4() != nil {
		return "ip4:icmp", 1, ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply
	}
	return "ip6:ipv6-icmp", 58, ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply
}

func probeCount(count int) int {
	if count > 0 {
		return count
	}
	return 1
}
