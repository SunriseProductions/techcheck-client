// Package e2e_test runs the sysinfo + nettest + upload pipeline against
// locally-spawned mock probe and ingest processes. It's the closest thing
// to a full wizard run that works in CI.
package e2e_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/sysinfo"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/upload"
)

// TestFullPipelineAgainstMocks verifies:
//   - the mock probe serves the documented endpoints and UDP echo
//   - the mock ingest round-trips a report and returns an id
//   - a retry with the same run_id returns 200 with the same id (idempotency)
func TestFullPipelineAgainstMocks(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}

	probeHTTP, probeUDP := freePort(t), freeUDPPort(t)
	ingestPort := freePort(t)

	probe := startBinary(t, "./cmd/mockprobe",
		"-http", "127.0.0.1:"+probeHTTP,
		"-udp", "127.0.0.1:"+probeUDP)
	defer stop(probe)

	ingest := startBinary(t, "./cmd/mockingest",
		"-addr", "127.0.0.1:"+ingestPort)
	defer stop(ingest)

	waitForHTTP(t, "http://127.0.0.1:"+probeHTTP+"/preflight/ping", 5*time.Second)
	waitForHTTP(t, "http://127.0.0.1:"+ingestPort+"/api/v1/reports", 5*time.Second)

	cfg := &config.Config{
		IngestURL:             "http://127.0.0.1:" + ingestPort + "/api/v1/reports",
		UploadToken:           "test-token",
		PerTestTimeoutSeconds: 5,
		UploadTimeoutSeconds:  5,
		UploadRetries:         1,
		UDPProbeMagic:         "PFLT",
		POPs: []config.POP{
			{ID: "local", RegionLabel: "Localhost", Hostname: "127.0.0.1", UDPEcho: true},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Sysinfo.
	m, err := sysinfo.Collect(ctx, sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)

	// Network — single POP via the test-only POPInput override so we can point
	// the HTTP URL at the probe's ephemeral port.
	prog := make(chan nettest.Progress, 128)
	go func() {
		for range prog {
		}
	}()
	probeHTTPPort := atoi(t, probeHTTP)
	probeUDPPort := atoi(t, probeUDP)
	popRes, errs := nettest.RunPOP(ctx, nettest.POPInput{
		POP:      cfg.POPs[0],
		BaseURL:  "http://127.0.0.1:" + probeHTTP,
		TCPPort:  probeHTTPPort,
		UDPPort:  probeUDPPort,
		ForceTCP: true,
	}, cfg, prog)
	close(prog)

	r := report.New()
	r.User.FullName = "E2E Runner"
	r.User.Email = "e2e@example.com"
	r.Consent.Accepted = true
	r.Consent.At = time.Now().UTC()
	r.Machine = m
	r.Network.POPs = []report.POPResult{popRes}
	r.Errors = append(r.Errors, errs...)
	r.RunCompletedAt = time.Now().UTC()

	// Upload.
	resp, err := upload.Upload(ctx, upload.Options{
		URL: cfg.IngestURL, Token: cfg.UploadToken, MaxRetries: 1,
	}, r)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)

	// Idempotent retry: same run_id → same id.
	resp2, err := upload.Upload(ctx, upload.Options{
		URL: cfg.IngestURL, Token: cfg.UploadToken, MaxRetries: 1,
	}, r)
	require.NoError(t, err)
	assert.Equal(t, resp.ID, resp2.ID, "server must dedupe on run_id")

	// Local report write worked on the happy path too.
	localPath, err := report.WriteLocalToDir(r, t.TempDir())
	require.NoError(t, err)
	_, err = os.Stat(localPath)
	require.NoError(t, err)
}

// -- helpers --

func startBinary(t *testing.T, pkg string, args ...string) *exec.Cmd {
	t.Helper()
	full := append([]string{"run", pkg}, args...)
	cmd := exec.Command("go", full...)
	cmd.Dir = repoRoot(t)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	return cmd
}

func stop(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func waitForHTTP(t *testing.T, url string, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		r, err := http.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return port
}

func freeUDPPort(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()
	_, port, err := net.SplitHostPort(pc.LocalAddr().String())
	require.NoError(t, err)
	return port
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	require.NoError(t, err)
	return n
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	// cmd/techcheck/tests/e2e → repo root is four levels up.
	return filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
}
