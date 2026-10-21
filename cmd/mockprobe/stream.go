package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"
)

// streamCookie is the fixed cookie the mock hands out. The real beacon binds
// the cookie to the client address; the mock only checks that S repeats it.
var streamCookie = []byte("mockprobe-cookie")

// helloBodyLen is ver u8, nonce [8], rate_kbps u32, packet_bytes u16,
// duration_ms u32.
const helloBodyLen = 19

// serveUDP answers the UDP stream protocol on pc and echoes every other
// packet that starts with magic. It returns when pc is closed.
func serveUDP(pc net.PacketConn, magic string) {
	buf := make([]byte, 1500)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < len(magic) || string(buf[:len(magic)]) != magic {
			continue
		}
		pkt := append([]byte(nil), buf[:n]...)
		if n < len(magic)+1 {
			_, _ = pc.WriteTo(pkt, addr)
			continue
		}
		body := pkt[len(magic)+1:]
		switch pkt[len(magic)] {
		case 'H':
			if len(body) < helloBodyLen {
				continue
			}
			reply := append([]byte(magic+"C"), body[1:9]...)
			reply = append(reply, streamCookie...)
			_, _ = pc.WriteTo(reply, addr)
		case 'S':
			if len(body) < helloBodyLen+len(streamCookie) ||
				!bytes.Equal(body[helloBodyLen:helloBodyLen+len(streamCookie)], streamCookie) {
				continue
			}
			nonce := append([]byte(nil), body[1:9]...)
			rateKbps := binary.BigEndian.Uint32(body[9:13])
			packetBytes := int(binary.BigEndian.Uint16(body[13:15]))
			duration := time.Duration(binary.BigEndian.Uint32(body[15:19])) * time.Millisecond
			go sendStream(pc, addr, magic, nonce, rateKbps, packetBytes, duration)
		default:
			_, _ = pc.WriteTo(pkt, addr)
		}
	}
}

// sendStream paces D packets at rateKbps for duration, then sends E three
// times, 20 ms apart. It sends in bursts so coarse timers still hold the
// average rate.
func sendStream(pc net.PacketConn, addr net.Addr, magic string, nonce []byte, rateKbps uint32, packetBytes int, duration time.Duration) {
	packetBytes = min(max(packetBytes, 200), 1400)
	pps := float64(rateKbps) * 1000 / float64(packetBytes*8)
	total := int(pps * duration.Seconds())

	pkt := make([]byte, packetBytes)
	copy(pkt, magic+"D")
	copy(pkt[len(magic)+1:], nonce)
	seqOff := len(magic) + 1 + len(nonce)
	start := time.Now()
	for seq := 0; seq < total; {
		due := min(int(time.Since(start).Seconds()*pps)+1, total)
		for ; seq < due; seq++ {
			binary.BigEndian.PutUint32(pkt[seqOff:], uint32(seq))
			binary.BigEndian.PutUint64(pkt[seqOff+4:], uint64(time.Now().UnixNano()))
			if _, err := pc.WriteTo(pkt, addr); err != nil {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}

	end := append([]byte(magic+"E"), nonce...)
	end = binary.BigEndian.AppendUint32(end, uint32(total))
	for i := 0; i < 3; i++ {
		if _, err := pc.WriteTo(end, addr); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
