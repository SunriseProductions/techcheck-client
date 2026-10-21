package nettest_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return ln
}

func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return host, port
}

// acceptLoop drains incoming TCP connections so DialContext succeeds.
func acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	require.NoError(t, err)
	return n
}

// streamMode picks how streamServer answers the UDP stream protocol.
type streamMode string

const (
	// streamNormal answers H with C and streams D packets after a valid S.
	streamNormal streamMode = "normal"
	// streamEchoOnly echoes every PFLT packet back, like beacons from before
	// the stream test.
	streamEchoOnly streamMode = "echoOnly"
	// streamBusy answers H with C and every S with B and a 1 s retry_after,
	// as a beacon does when its bandwidth slots are full.
	streamBusy streamMode = "busy"
	// streamBusyOnce answers the first S with B, then streams normally.
	streamBusyOnce streamMode = "busyOnce"
	// streamDropFirstHello ignores the first H, as if the line lost it, then
	// behaves like streamNormal.
	streamDropFirstHello streamMode = "dropFirstHello"
	// streamSilentAfterStart completes the handshake and then sends nothing,
	// as when the S is lost on the way.
	streamSilentAfterStart streamMode = "silentAfterStart"
	// streamNoData completes the handshake and sends E, but no D packets, as
	// when large datagrams are dropped on the path.
	streamNoData streamMode = "noData"
	// streamLossyAbove10 is streamNormal, but skips every 20th packet (5 %)
	// when the rate is above 10 Mbps.
	streamLossyAbove10 streamMode = "lossyAbove10"
	// streamLateData is streamNormal, but holds back the last D packet until
	// 10 ms after the first E, as when the path reorders it.
	streamLateData streamMode = "lateData"
)

// testStreamServer is a UDP beacon that speaks the stream wire format.
type testStreamServer struct {
	host   string
	port   int
	hellos atomic.Int32 // every H received
	starts atomic.Int32 // every S with a valid cookie
}

// streamServer starts a beacon on 127.0.0.1 in the given mode. It closes
// when the test ends.
func streamServer(t *testing.T, mode streamMode) *testStreamServer {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })

	host, port := hostPort(t, pc.LocalAddr().String())
	srv := &testStreamServer{host: host, port: port}
	cookie := make([]byte, 16)
	_, err = rand.Read(cookie)
	require.NoError(t, err)

	const helloBodyLen = 19 // ver, nonce, rate_kbps, packet_bytes, duration_ms
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 5 || string(buf[:4]) != "PFLT" {
				continue
			}
			pkt := append([]byte(nil), buf[:n]...)
			if mode == streamEchoOnly {
				_, _ = pc.WriteTo(pkt, addr)
				continue
			}
			body := pkt[5:]
			switch pkt[4] {
			case 'H':
				if len(body) < helloBodyLen {
					continue
				}
				if srv.hellos.Add(1) == 1 && mode == streamDropFirstHello {
					continue
				}
				nonce := body[1:9]
				reply := append([]byte("PFLTC"), nonce...)
				reply = append(reply, cookie...)
				_, _ = pc.WriteTo(reply, addr)
			case 'S':
				if len(body) < helloBodyLen+16 || !bytes.Equal(body[helloBodyLen:helloBodyLen+16], cookie) {
					continue
				}
				starts := srv.starts.Add(1)
				nonce := append([]byte(nil), body[1:9]...)
				if mode == streamBusy || (mode == streamBusyOnce && starts == 1) {
					reply := append([]byte("PFLTB"), nonce...)
					reply = binary.BigEndian.AppendUint16(reply, 1)
					_, _ = pc.WriteTo(reply, addr)
					continue
				}
				if mode == streamSilentAfterStart {
					continue
				}
				rateKbps := binary.BigEndian.Uint32(body[9:13])
				packetBytes := int(binary.BigEndian.Uint16(body[13:15]))
				duration := time.Duration(binary.BigEndian.Uint32(body[15:19])) * time.Millisecond
				go sendStream(pc, addr, mode, nonce, rateKbps, packetBytes, duration)
			default:
				// Any other PFLT packet is a plain echo probe.
				_, _ = pc.WriteTo(pkt, addr)
			}
		}
	}()
	return srv
}

// sendStream paces D packets at rateKbps for duration, then sends E three
// times, 20 ms apart. It sends in bursts so coarse timers (Windows) still
// hold the average rate.
func sendStream(pc net.PacketConn, addr net.Addr, mode streamMode, nonce []byte, rateKbps uint32, packetBytes int, duration time.Duration) {
	if packetBytes < 25 {
		packetBytes = 25
	}
	pps := float64(rateKbps) * 1000 / float64(packetBytes*8)
	total := int(pps * duration.Seconds())
	drop := mode == streamLossyAbove10 && rateKbps > 10_000

	if mode == streamNoData {
		time.Sleep(duration)
	} else {
		pkt := make([]byte, packetBytes)
		copy(pkt, "PFLTD")
		copy(pkt[5:], nonce)
		start := time.Now()
		for seq := 0; seq < total; {
			due := min(int(time.Since(start).Seconds()*pps)+1, total)
			for ; seq < due; seq++ {
				if drop && seq%20 == 0 {
					continue
				}
				if mode == streamLateData && seq == total-1 {
					continue
				}
				binary.BigEndian.PutUint32(pkt[13:], uint32(seq))
				binary.BigEndian.PutUint64(pkt[17:], uint64(time.Now().UnixNano()))
				if _, err := pc.WriteTo(pkt, addr); err != nil {
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}

	end := append([]byte("PFLTE"), nonce...)
	end = binary.BigEndian.AppendUint32(end, uint32(total))
	for i := 0; i < 3; i++ {
		if _, err := pc.WriteTo(end, addr); err != nil {
			return
		}
		if i == 0 && mode == streamLateData {
			time.Sleep(10 * time.Millisecond)
			late := make([]byte, packetBytes)
			copy(late, "PFLTD")
			copy(late[5:], nonce)
			binary.BigEndian.PutUint32(late[13:], uint32(total-1))
			binary.BigEndian.PutUint64(late[17:], uint64(time.Now().UnixNano()))
			if _, err := pc.WriteTo(late, addr); err != nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
