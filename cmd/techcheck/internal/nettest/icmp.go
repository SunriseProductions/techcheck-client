package nettest

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// icmpv4ProtocolNumber is the IANA protocol number for ICMPv4, required by
// icmp.ParseMessage to decode replies read from the unprivileged socket.
const icmpv4ProtocolNumber = 1

// icmpPings runs n ICMP echo requests against host using an unprivileged
// ICMP socket ("udp4"). Requires OS support (macOS + recent Linux); Windows
// does not support unprivileged ICMP and the caller is expected to fall back
// to TCP. Returns the successful RTT samples (ms) and the count of dropped
// attempts.
func icmpPings(ctx context.Context, host string, n int, to time.Duration) ([]float64, int, error) {
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, 0, fmt.Errorf("icmp listen: %w", err)
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve %s: %w", host, err)
	}

	echoID := os.Getpid() & 0xffff
	samples := make([]float64, 0, n)
	for seq := 1; seq <= n; seq++ {
		if err := ctx.Err(); err != nil {
			return samples, n - len(samples), err
		}
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Body: &icmp.Echo{ID: echoID, Seq: seq, Data: []byte("PFLT")},
		}
		b, err := msg.Marshal(nil)
		if err != nil {
			return samples, n - len(samples), fmt.Errorf("marshal icmp: %w", err)
		}

		if err := c.SetDeadline(time.Now().Add(to)); err != nil {
			return samples, n - len(samples), fmt.Errorf("set deadline: %w", err)
		}
		start := time.Now()
		if _, err := c.WriteTo(b, &net.UDPAddr{IP: dst.IP}); err != nil {
			continue
		}
		reply := make([]byte, 1500)
		// Keep reading until we get a reply matching our (ID, Seq) or the
		// deadline fires. This defends against unrelated ICMP traffic on the
		// socket.
		for {
			nBytes, _, err := c.ReadFrom(reply)
			if err != nil {
				break
			}
			parsed, perr := icmp.ParseMessage(icmpv4ProtocolNumber, reply[:nBytes])
			if perr != nil {
				continue
			}
			echo, ok := parsed.Body.(*icmp.Echo)
			if !ok || echo.ID != echoID || echo.Seq != seq {
				continue
			}
			samples = append(samples, float64(time.Since(start).Microseconds())/1000.0)
			break
		}
	}
	dropped := n - len(samples)
	if len(samples) == 0 {
		return nil, dropped, fmt.Errorf("all icmp pings failed")
	}
	return samples, dropped, nil
}
