package nettest

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

type UDPEchoInput struct {
	Host    string
	Port    int
	Magic   string        // "PFLT" per PRD §6.2
	Timeout time.Duration // defaults to 3s
}

// MeasureUDPEcho sends a datagram prefixed with the magic bytes + a random
// 8-byte nonce, and waits for the same payload to echo back. The nonce check
// ensures we don't accept a stray UDP packet from another source as a valid
// reply.
func MeasureUDPEcho(ctx context.Context, in UDPEchoInput) (report.Reachability, error) {
	if in.Timeout <= 0 {
		in.Timeout = 3 * time.Second
	}
	if in.Magic == "" {
		in.Magic = "PFLT"
	}

	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}, err
	}
	// Payload layout: magic + 0x00 + nonce. The server echoes the bytes verbatim.
	prefix := append([]byte(in.Magic), 0x00)
	payload := append(prefix, nonce...)

	addr := &net.UDPAddr{IP: net.ParseIP(in.Host), Port: in.Port}
	if addr.IP == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", in.Host)
		if err != nil || len(ips) == 0 {
			return report.Reachability{Status: report.ReachabilityBlocked}, fmt.Errorf("resolve: %w", err)
		}
		addr.IP = ips[0]
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(in.Timeout)); err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}, err
	}
	if _, err := conn.Write(payload); err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}, err
	}
	buf := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}, err
	}
	if n < len(payload) || !bytes.Equal(buf[:len(payload)], payload) {
		return report.Reachability{Status: report.ReachabilityBlocked}, fmt.Errorf("echo payload mismatch")
	}
	return report.Reachability{Status: report.ReachabilityReachable}, nil
}
