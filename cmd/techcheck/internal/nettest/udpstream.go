package nettest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// ErrStreamUnsupported means the beacon does not speak the stream protocol:
// it echoed the hello back, as beacons from before the stream test do, or no
// cookie came back within the timeout.
var ErrStreamUnsupported = errors.New("stream not supported")

type StreamInput struct {
	Host        string
	Port        int
	Magic       string // "PFLT"
	RateMbps    float64
	PacketBytes int
	Duration    time.Duration
	Timeout     time.Duration // wait for the cookie; defaults to 3s
}

const (
	streamVersion      = 1
	streamReadBuffer   = 4 << 20
	streamEndGrace     = time.Second
	streamBusyFallback = 5 * time.Second
	streamNonceLen     = 8
	streamCookieLen    = 16
)

// MeasureUDPStream asks the beacon to stream sequenced packets at RateMbps
// for Duration and summarises what arrived. anyData is false when the
// handshake worked but no data packet arrived.
//
// The handshake is H (hello) → C (cookie) → S (start). S goes out exactly
// once: the beacon starts a new stream for every valid S, so a resend would
// double the stream. A B (busy) reply is retried up to bwMaxAttempts times,
// waiting the beacon's retry_after between attempts.
func MeasureUDPStream(ctx context.Context, in StreamInput) (report.StreamStep, bool, error) {
	if in.Timeout <= 0 {
		in.Timeout = 3 * time.Second
	}
	if in.Magic == "" {
		in.Magic = "PFLT"
	}
	var lastErr error
	for attempt := 0; attempt < bwMaxAttempts; attempt++ {
		step, anyData, err := measureStreamOnce(ctx, in)
		if err == nil {
			return step, anyData, nil
		}
		var be *busyError
		if !errors.As(err, &be) {
			return report.StreamStep{}, false, err
		}
		lastErr = err
		if attempt == bwMaxAttempts-1 {
			break
		}
		if waitErr := waitForRetry(ctx, be.retryAfter); waitErr != nil {
			return report.StreamStep{}, false, waitErr
		}
	}
	return report.StreamStep{}, false, fmt.Errorf("stream still busy after %d attempts: %w", bwMaxAttempts, lastErr)
}

func measureStreamOnce(ctx context.Context, in StreamInput) (report.StreamStep, bool, error) {
	addr := &net.UDPAddr{IP: net.ParseIP(in.Host), Port: in.Port}
	if addr.IP == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", in.Host)
		if err != nil || len(ips) == 0 {
			return report.StreamStep{}, false, fmt.Errorf("resolve: %w", err)
		}
		addr.IP = ips[0]
	}

	// One connected socket carries the handshake and the data, so the data
	// comes back to the source port the beacon bound the cookie to.
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return report.StreamStep{}, false, err
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(streamReadBuffer) // best effort; the OS may cap it
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	nonce := make([]byte, streamNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return report.StreamStep{}, false, err
	}
	magic := []byte(in.Magic)
	body := streamHelloBody(nonce, in)

	if _, err := conn.Write(streamPacket(magic, 'H', body)); err != nil {
		return report.StreamStep{}, false, err
	}
	cookie, err := awaitStreamCookie(ctx, conn, magic, nonce, in.Timeout)
	if err != nil {
		return report.StreamStep{}, false, err
	}

	start := append(append([]byte(nil), body...), cookie...)
	if _, err := conn.Write(streamPacket(magic, 'S', start)); err != nil {
		return report.StreamStep{}, false, err
	}
	pkts, totalSent, err := receiveStream(ctx, conn, magic, nonce, time.Now().Add(in.Duration+streamEndGrace))
	if err != nil {
		return report.StreamStep{}, false, err
	}
	return StreamStats(in.RateMbps, totalSent, pkts), len(pkts) > 0, nil
}

// streamHelloBody is the H body, which S repeats before the cookie:
// ver u8, nonce [8], rate_kbps u32, packet_bytes u16, duration_ms u32.
func streamHelloBody(nonce []byte, in StreamInput) []byte {
	b := make([]byte, 0, 19)
	b = append(b, streamVersion)
	b = append(b, nonce...)
	b = binary.BigEndian.AppendUint32(b, uint32(math.Round(in.RateMbps*1000)))
	b = binary.BigEndian.AppendUint16(b, uint16(in.PacketBytes))
	b = binary.BigEndian.AppendUint32(b, uint32(in.Duration.Milliseconds()))
	return b
}

func streamPacket(magic []byte, typ byte, body []byte) []byte {
	p := make([]byte, 0, len(magic)+1+len(body))
	p = append(p, magic...)
	p = append(p, typ)
	return append(p, body...)
}

// parseStreamPacket splits a packet into its type and body. ok is false when
// the packet does not start with magic.
func parseStreamPacket(p, magic []byte) (typ byte, body []byte, ok bool) {
	if len(p) < len(magic)+1 || !bytes.Equal(p[:len(magic)], magic) {
		return 0, nil, false
	}
	return p[len(magic)], p[len(magic)+1:], true
}

// awaitStreamCookie reads the reply to a hello. A C with our nonce returns
// the cookie; a B returns a busyError; an echoed H, or nothing within
// timeout, returns ErrStreamUnsupported.
func awaitStreamCookie(ctx context.Context, conn *net.UDPConn, magic, nonce []byte, timeout time.Duration) ([]byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	buf := make([]byte, 2048)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return nil, fmt.Errorf("no stream cookie within %s: %w", timeout, ErrStreamUnsupported)
			}
			// On a connected socket this is an ICMP error, such as port
			// unreachable: nothing there speaks the stream protocol.
			return nil, fmt.Errorf("%w: %w", ErrStreamUnsupported, err)
		}
		typ, body, ok := parseStreamPacket(buf[:n], magic)
		if !ok {
			continue
		}
		switch typ {
		case 'H':
			return nil, fmt.Errorf("beacon echoed the hello: %w", ErrStreamUnsupported)
		case 'C':
			if len(body) >= streamNonceLen+streamCookieLen && bytes.Equal(body[:streamNonceLen], nonce) {
				return append([]byte(nil), body[streamNonceLen:streamNonceLen+streamCookieLen]...), nil
			}
		case 'B':
			if len(body) >= streamNonceLen+2 && bytes.Equal(body[:streamNonceLen], nonce) {
				retryAfter := time.Duration(binary.BigEndian.Uint16(body[streamNonceLen:])) * time.Second
				if retryAfter <= 0 {
					retryAfter = streamBusyFallback
				}
				return nil, &busyError{retryAfter: retryAfter}
			}
		}
	}
}

// receiveStream collects D packets until an E arrives or the deadline
// passes. It returns the total_sent from E, or 0 when no E arrived. Packets
// of other types or with another nonce are ignored.
func receiveStream(ctx context.Context, conn *net.UDPConn, magic, nonce []byte, deadline time.Time) ([]StreamPacket, int, error) {
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 64<<10)
	var pkts []StreamPacket
	for {
		n, err := conn.Read(buf)
		recvNS := time.Now().UnixNano()
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return pkts, 0, nil
			}
			return nil, 0, err
		}
		typ, body, ok := parseStreamPacket(buf[:n], magic)
		if !ok || len(body) < streamNonceLen || !bytes.Equal(body[:streamNonceLen], nonce) {
			continue
		}
		rest := body[streamNonceLen:]
		switch typ {
		case 'D':
			if len(rest) < 12 {
				continue
			}
			pkts = append(pkts, StreamPacket{
				Seq:    binary.BigEndian.Uint32(rest[0:4]),
				SendNS: int64(binary.BigEndian.Uint64(rest[4:12])),
				RecvNS: recvNS,
			})
		case 'E':
			if len(rest) < 4 {
				continue
			}
			return pkts, int(binary.BigEndian.Uint32(rest[0:4])), nil
		}
	}
}

// RunLadder runs one stream per rate in ladderMbps, each for step, and stops
// at the first step whose loss is above lossLimitPct or that got no data at
// all. MaxCleanMbps is the rate of the last step at or under the limit (0 if
// none). LargePackets comes from the first step: dropped when its handshake
// worked but no data packet arrived.
//
// An error from a step (unsupported beacon, still busy) ends the ladder; the
// steps completed so far come back with it.
func RunLadder(ctx context.Context, in StreamInput, ladderMbps []float64, step time.Duration, lossLimitPct float64) (report.StreamLadder, error) {
	ladder := report.StreamLadder{Port: in.Port, Steps: []report.StreamStep{}}
	for i, rate := range ladderMbps {
		run := in
		run.RateMbps = rate
		run.Duration = step
		st, anyData, err := MeasureUDPStream(ctx, run)
		if err != nil {
			return ladder, err
		}
		if i == 0 {
			ladder.LargePackets = report.LargePacketsOK
			if !anyData {
				ladder.LargePackets = report.LargePacketsDropped
			}
		}
		ladder.Steps = append(ladder.Steps, st)
		if !anyData || st.LossPct > lossLimitPct {
			break
		}
		ladder.MaxCleanMbps = rate
	}
	return ladder, nil
}
