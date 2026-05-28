package nettest_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// TestRunPOPHappyPath stands up a mock probe-target nginx (all six endpoints)
// plus a UDP echo sidecar, then runs RunPOP against it and asserts the
// aggregated result.
func TestRunPOPHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/preflight/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/preflight/download/10mb", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1<<19))
	})
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/preflight/mtu", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1400)) })
	mux.HandleFunc("/preflight/echo-ts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"iso": time.Now().UTC().Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("/preflight/whoami", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"public_ip": "127.0.0.1", "pop_id": "local", "geo": "Localhost"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	udpLn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udpLn.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := udpLn.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udpLn.WriteTo(buf[:n], addr)
		}
	}()

	host, port := hostPort(t, srv.Listener.Addr().String())
	_, udpPortStr, _ := net.SplitHostPort(udpLn.LocalAddr().String())
	udpPort := atoi(t, udpPortStr)

	pop := config.POP{ID: "local", RegionLabel: "Localhost", Hostname: host, UDPEcho: true}
	cfg := &config.Config{
		PerTestTimeoutSeconds: 3,
		UDPProbeMagic:         "PFLT",
	}

	progress := make(chan nettest.Progress, 128)
	progressDone := make(chan struct{})
	var progressCount int
	go func() {
		for range progress {
			progressCount++
		}
		close(progressDone)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res, errs := nettest.RunPOP(ctx, nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: udpPort, ForceTCP: true,
	}, cfg, progress)
	close(progress)
	<-progressDone

	assert.Equal(t, "local", res.ID)
	assert.Equal(t, report.LatencyMethodTCP, res.Latency.Method)
	assert.Equal(t, report.MTUStatusPass, res.MTU.Status)
	assert.Equal(t, report.ReachabilityReachable, res.TCP443.Status)
	assert.Equal(t, report.ReachabilityReachable, res.UDP4172.Status)
	assert.Empty(t, errs, "no per-test errors on happy path")
	assert.Greater(t, progressCount, 6, "expected progress events for each test")
}

// TestRunPOPRespectsUDPEchoFlag sets UDPEcho=false and verifies the UDP probe
// is skipped (result stays Blocked without attempting the socket).
func TestRunPOPRespectsUDPEchoFlag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/preflight/download/10mb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1<<18)) })
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/preflight/mtu", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1400)) })
	mux.HandleFunc("/preflight/echo-ts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"iso": time.Now().UTC().Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("/preflight/whoami", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"public_ip": "127.0.0.1", "pop_id": "local", "geo": "x"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	host, port := hostPort(t, srv.Listener.Addr().String())

	pop := config.POP{ID: "nopop", RegionLabel: "NoUDP", Hostname: host, UDPEcho: false}
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT"}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res, _ := nettest.RunPOP(ctx, nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: 1, ForceTCP: true,
	}, cfg, nil)

	assert.Equal(t, report.ReachabilityBlocked, res.UDP4172.Status)
}
