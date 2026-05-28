package nettest_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestUDPEchoReachable(t *testing.T) {
	// Local UDP echo server that only replies to packets starting with "PFLT".
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 4 || string(buf[:4]) != "PFLT" {
				continue
			}
			_, _ = ln.WriteTo(buf[:n], addr)
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.LocalAddr().String())

	res, err := nettest.MeasureUDPEcho(context.Background(), nettest.UDPEchoInput{
		Host:    "127.0.0.1",
		Port:    atoi(t, portStr),
		Magic:   "PFLT",
		Timeout: 2 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, report.ReachabilityReachable, res.Status)
}

func TestUDPEchoBlockedOnTimeout(t *testing.T) {
	// Listener that drops all packets.
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			_, _, err := ln.ReadFrom(buf)
			if err != nil {
				return
			}
			// Drop.
		}
	}()
	_, portStr, _ := net.SplitHostPort(ln.LocalAddr().String())

	res, _ := nettest.MeasureUDPEcho(context.Background(), nettest.UDPEchoInput{
		Host:    "127.0.0.1",
		Port:    atoi(t, portStr),
		Magic:   "PFLT",
		Timeout: 200 * time.Millisecond,
	})
	assert.Equal(t, report.ReachabilityBlocked, res.Status)
}

func TestUDPEchoRejectsNonceMismatch(t *testing.T) {
	// Listener that echoes with a garbled nonce.
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				return
			}
			// Flip the last byte of the nonce before echoing.
			if n >= 13 {
				buf[n-1] ^= 0xff
			}
			_, _ = ln.WriteTo(buf[:n], addr)
		}
	}()
	_, portStr, _ := net.SplitHostPort(ln.LocalAddr().String())

	res, err := nettest.MeasureUDPEcho(context.Background(), nettest.UDPEchoInput{
		Host:    "127.0.0.1",
		Port:    atoi(t, portStr),
		Magic:   "PFLT",
		Timeout: 500 * time.Millisecond,
	})
	assert.Error(t, err)
	assert.Equal(t, report.ReachabilityBlocked, res.Status)
}
