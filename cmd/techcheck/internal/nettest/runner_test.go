package nettest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		time.Sleep(loopbackDelay)
		_, _ = w.Write(make([]byte, 1<<19))
	})
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(loopbackDelay)
		w.WriteHeader(200)
	})
	mux.HandleFunc("/preflight/mtu", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1400)) })
	mux.HandleFunc("/preflight/echo-ts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"iso": time.Now().UTC().Format(time.RFC3339Nano)})
	})
	mux.HandleFunc("/preflight/whoami", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"public_ip": "127.0.0.1", "pop_id": "local", "geo": "Localhost"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	udp := streamServer(t, streamNormal)

	host, port := hostPort(t, srv.Listener.Addr().String())

	pop := config.POP{ID: "local", RegionLabel: "Localhost", Hostname: host, UDPEcho: true}
	cfg := &config.Config{
		PerTestTimeoutSeconds: 3,
		UDPProbeMagic:         "PFLT",
		Run:                   tinyRunSettings(),
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
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: udp.port, DCVUDPPort: udp.port,
		ForceTCP: true, ClockSkew: true,
	}, cfg, progress)
	close(progress)
	<-progressDone

	assert.Equal(t, "local", res.ID)
	assert.Equal(t, report.LatencyMethodTCP, res.Latency.Method)
	assert.Equal(t, report.MTUStatusPass, res.MTU.Status)
	assert.Equal(t, report.ReachabilityReachable, res.TCP443.Status)
	assert.Equal(t, report.ReachabilityReachable, res.UDP4172.Status)
	assert.Equal(t, report.DepthFull, res.Depth)
	require.NotNil(t, res.UDP8443)
	assert.Equal(t, report.ReachabilityReachable, res.UDP8443.Status)
	require.NotNil(t, res.UDPStream)
	require.NotNil(t, res.UDPStream.Teradici)
	require.NotNil(t, res.UDPStream.DCV)
	assert.NotNil(t, res.Throughput)
	assert.NotNil(t, res.Jitter)
	assert.NotNil(t, res.Loss)
	assert.NotNil(t, res.ClockSkewMS)
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

// tinyRunSettings keeps the stream tests short: a 1 and 2 Mbps ladder with
// 200 ms steps, and a 200 ms Teradici run.
func tinyRunSettings() config.RunSettings {
	run := config.DefaultRunSettings()
	run.UDPStream.LadderMbps = []float64{1, 2}
	run.UDPStream.StepSeconds = 0.2
	run.UDPStream.TeradiciMbps = 1
	run.UDPStream.TeradiciSeconds = 0.2
	return run
}

// quickProbeServer serves the HTTP endpoints FullPOP needs.
func quickProbeServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/preflight/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/preflight/download/10mb", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(loopbackDelay)
		_, _ = w.Write(make([]byte, 1<<19))
	})
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(loopbackDelay)
		w.WriteHeader(200)
	})
	mux.HandleFunc("/preflight/mtu", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1400)) })
	mux.HandleFunc("/preflight/echo-ts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"iso": time.Now().UTC().Format(time.RFC3339Nano)})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func collectProgress(ch chan nettest.Progress) []nettest.Progress {
	close(ch)
	var rows []nettest.Progress
	for p := range ch {
		rows = append(rows, p)
	}
	return rows
}

func TestQuickPOPRecordsPortsOnly(t *testing.T) {
	srv := quickProbeServer(t)
	udp := streamServer(t, streamNormal)
	host, port := hostPort(t, srv.Listener.Addr().String())

	pki := newTestPKI(t)
	_, tlsPort := startTLSServer(t, pki)

	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	cfg.Run.TLSExpectedRoots = []string{"Test Root"}
	pop := config.POP{ID: "q", RegionLabel: "Quick", Hostname: host, UDPEcho: true}

	progress := make(chan nettest.Progress, 128)
	res, errs := nettest.QuickPOP(context.Background(), nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: udp.port,
		DCVTCPPort: tlsPort, DCVUDPPort: udp.port, TLSRoots: pki.pool(), ForceTCP: true,
	}, cfg, progress)
	rows := collectProgress(progress)

	assert.Empty(t, errs)
	assert.Equal(t, report.DepthQuick, res.Depth)
	assert.Equal(t, report.LatencyMethodTCP, res.Latency.Method)
	assert.Equal(t, report.ReachabilityReachable, res.TCP443.Status)
	assert.Equal(t, report.ReachabilityReachable, res.UDP4172.Status)
	require.NotNil(t, res.TCP8443)
	assert.Equal(t, report.ReachabilityReachable, res.TCP8443.Status)
	assert.False(t, res.TCP8443.TLSIntercepted)
	require.NotNil(t, res.UDP8443)
	assert.Equal(t, report.ReachabilityReachable, res.UDP8443.Status)

	assert.Nil(t, res.Throughput)
	assert.Nil(t, res.Jitter)
	assert.Nil(t, res.Loss)
	assert.Nil(t, res.MTU)
	assert.Nil(t, res.ClockSkewMS)
	assert.Nil(t, res.UDPStream)

	var quick []nettest.Progress
	for _, r := range rows {
		assert.Equal(t, "quick", r.Test)
		if r.Phase == "done" {
			quick = append(quick, r)
		}
	}
	require.Len(t, quick, 1)
	assert.Regexp(t, `^\d+ ms$`, quick[0].Summary)
}

func TestQuickPOPUDPEchoFalseBlocksBothWithoutError(t *testing.T) {
	srv := quickProbeServer(t)
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pop := config.POP{ID: "q", Hostname: host, UDPEcho: false}

	res, errs := nettest.QuickPOP(context.Background(), nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: 1, DCVUDPPort: 1, DCVTCPPort: 1, ForceTCP: true,
	}, cfg, nil)

	assert.Equal(t, report.ReachabilityBlocked, res.UDP4172.Status)
	require.NotNil(t, res.UDP8443)
	assert.Equal(t, report.ReachabilityBlocked, res.UDP8443.Status)
	require.NotNil(t, res.TCP8443)
	assert.Equal(t, report.ReachabilityBlocked, res.TCP8443.Status)
	assert.Empty(t, errs)
}

func TestQuickPOPLatencyFailureIsFailedRow(t *testing.T) {
	cfg := &config.Config{PerTestTimeoutSeconds: 1, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pop := config.POP{ID: "dead", Hostname: "127.0.0.1"}

	progress := make(chan nettest.Progress, 128)
	res, errs := nettest.QuickPOP(context.Background(), nettest.POPInput{
		POP: pop, BaseURL: "http://127.0.0.1:1", TCPPort: 1, UDPPort: 1, DCVUDPPort: 1, DCVTCPPort: 1, ForceTCP: true,
	}, cfg, progress)
	rows := collectProgress(progress)

	assert.Equal(t, report.DepthQuick, res.Depth)
	assert.NotEmpty(t, res.Latency.Method)
	var failed []nettest.Progress
	for _, r := range rows {
		if r.Phase == "failed" {
			failed = append(failed, r)
		}
	}
	require.Len(t, failed, 1)
	assert.Equal(t, "quick", failed[0].Test)
	assert.NotEmpty(t, failed[0].Err)
	require.NotEmpty(t, errs)
	assert.Equal(t, "latency", errs[0].Test)
}

// runFull runs QuickPOP then FullPOP against a loopback probe and a stream
// server in the given mode, which serves both UDP ports.
func runFull(t *testing.T, mode streamMode, in func(*nettest.POPInput)) (report.POPResult, []report.Error, []nettest.Progress) {
	t.Helper()
	srv := quickProbeServer(t)
	udp := streamServer(t, mode)
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pop := config.POP{ID: "f", RegionLabel: "Full", Hostname: host, UDPEcho: true}
	pin := nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: udp.port,
		DCVTCPPort: 1, DCVUDPPort: udp.port, ForceTCP: true, ClockSkew: true,
	}
	if in != nil {
		in(&pin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	quick, qerrs := nettest.QuickPOP(ctx, pin, cfg, nil)
	require.Empty(t, qerrs)
	progress := make(chan nettest.Progress, 256)
	res, errs := nettest.FullPOP(ctx, pin, cfg, quick, progress)
	return res, errs, collectProgress(progress)
}

func summaryOf(rows []nettest.Progress, test string) string {
	for _, r := range rows {
		if r.Test == test && r.Phase == "done" {
			return r.Summary
		}
	}
	return ""
}

func TestFullPOPAddsFullFieldsAndStreams(t *testing.T) {
	res, errs, rows := runFull(t, streamNormal, nil)

	assert.Empty(t, errs)
	assert.Equal(t, report.DepthFull, res.Depth)
	assert.NotNil(t, res.Throughput)
	assert.NotNil(t, res.Jitter)
	assert.NotNil(t, res.Loss)
	require.NotNil(t, res.MTU)
	assert.Equal(t, report.MTUStatusPass, res.MTU.Status)
	assert.NotNil(t, res.ClockSkewMS)
	require.NotNil(t, res.UDPStream)
	require.NotNil(t, res.UDPStream.Teradici)
	assert.NotNil(t, res.UDP8443)
	assert.Equal(t, report.LargePacketsOK, res.UDPStream.Teradici.LargePackets)
	assert.Greater(t, res.UDPStream.Teradici.Received, 0)
	require.NotNil(t, res.UDPStream.DCV)
	assert.Equal(t, report.LargePacketsOK, res.UDPStream.DCV.LargePackets)
	assert.Len(t, res.UDPStream.DCV.Steps, 2)

	assert.Regexp(t, `^\d+\.\d\d % loss$`, summaryOf(rows, "udp_stream_4172"))
	assert.Regexp(t, `^[\d.]+ Mbps clean, \d+\.\d\d % loss$`, summaryOf(rows, "udp_stream_8443"))
	assert.Equal(t, "reachable", summaryOf(rows, "udp_8443"))
	assert.Equal(t, "blocked", summaryOf(rows, "tcp_8443"))
}

func TestFullPOPStreamUnsupportedIsError(t *testing.T) {
	res, errs, _ := runFull(t, streamEchoOnly, nil)

	var tests []string
	for _, e := range errs {
		tests = append(tests, e.Test)
	}
	assert.Contains(t, tests, "udp_stream_8443")
	assert.Contains(t, tests, "udp_stream_4172")
	assert.Nil(t, res.UDPStream, "no stream result when neither stream ran")
	require.NotNil(t, res.UDP8443)
	assert.Equal(t, report.ReachabilityReachable, res.UDP8443.Status)
}

func TestFullPOPSkipsStreamsWhenUDPBlocked(t *testing.T) {
	srv := quickProbeServer(t)
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pop := config.POP{ID: "b", Hostname: host, UDPEcho: false}
	pin := nettest.POPInput{POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: 1, DCVUDPPort: 1, DCVTCPPort: 1, ForceTCP: true}

	quick, _ := nettest.QuickPOP(context.Background(), pin, cfg, nil)
	res, errs := nettest.FullPOP(context.Background(), pin, cfg, quick, nil)

	assert.Nil(t, res.UDPStream)
	for _, e := range errs {
		assert.NotContains(t, e.Test, "udp_stream")
	}
	assert.Nil(t, res.ClockSkewMS, "clock skew only when asked for")
}

func TestFullPOPKeepsDownloadWhenUploadFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/preflight/download/10mb", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(loopbackDelay)
		_, _ = w.Write(make([]byte, 1<<19))
	})
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pin := nettest.POPInput{
		POP: config.POP{ID: "d", Hostname: host}, BaseURL: srv.URL, TCPPort: port,
		UDPPort: 1, DCVUDPPort: 1, DCVTCPPort: 1, ForceTCP: true,
	}
	quick, _ := nettest.QuickPOP(context.Background(), pin, cfg, nil)
	res, errs := nettest.FullPOP(context.Background(), pin, cfg, quick, nil)

	require.NotNil(t, res.Throughput)
	require.NotNil(t, res.Throughput.DownMbps)
	assert.Nil(t, res.Throughput.UpMbps)
	var tests []string
	for _, e := range errs {
		tests = append(tests, e.Test)
	}
	assert.Contains(t, tests, "throughput_up")
	assert.NotContains(t, tests, "throughput_down")
	b, err := json.Marshal(res.Throughput)
	require.NoError(t, err)
	assert.Contains(t, string(b), "down_mbps")
	assert.NotContains(t, string(b), "up_mbps")
}

func TestFullPOPFailedMeasurementsStayNil(t *testing.T) {
	// No HTTP endpoints: throughput, jitter and clock skew fail.
	udp := streamServer(t, streamNormal)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	pop := config.POP{ID: "n", Hostname: host, UDPEcho: true}
	pin := nettest.POPInput{
		POP: pop, BaseURL: srv.URL, TCPPort: port, UDPPort: udp.port, DCVUDPPort: udp.port,
		DCVTCPPort: 1, ForceTCP: true, ClockSkew: true,
	}
	quick, _ := nettest.QuickPOP(context.Background(), pin, cfg, nil)
	res, errs := nettest.FullPOP(context.Background(), pin, cfg, quick, nil)

	assert.Nil(t, res.Throughput)
	assert.Nil(t, res.ClockSkewMS)
	var tests []string
	for _, e := range errs {
		tests = append(tests, e.Test)
	}
	assert.Contains(t, tests, "throughput_down")
	assert.Contains(t, tests, "clock_skew")
}

func quickAt(id string, median float64) report.POPResult {
	return report.POPResult{ID: id, Latency: report.LatencyResult{MedianMS: median}}
}

func TestSelectFullPOPs(t *testing.T) {
	cases := []struct {
		name  string
		quick []report.POPResult
		max   float64
		pops  int
		want  []string
	}{
		{"single near POP", []report.POPResult{quickAt("a", 20)}, 100, 3, []string{"a"}},
		{"two under 100 ms", []report.POPResult{quickAt("a", 30), quickAt("b", 60), quickAt("c", 150)}, 100, 3, []string{"a", "b"}},
		{"four under 100 ms capped at 3", []report.POPResult{
			quickAt("a", 10), quickAt("b", 20), quickAt("c", 30), quickAt("d", 40),
		}, 100, 3, []string{"a", "b", "c"}},
		{"nearest over 100 ms still chosen alone", []report.POPResult{quickAt("a", 180), quickAt("b", 220)}, 100, 3, []string{"a"}},
		{"ties ordered by id", []report.POPResult{quickAt("b", 20), quickAt("a", 20), quickAt("c", 20)}, 100, 2, []string{"a", "b"}},
		{"ascending latency", []report.POPResult{quickAt("a", 80), quickAt("b", 10), quickAt("c", 40)}, 100, 3, []string{"b", "c", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, nettest.SelectFullPOPs(tc.quick, nil, tc.max, tc.pops))
		})
	}
}

func TestSelectFullPOPsSkipsFailedLatency(t *testing.T) {
	quick := []report.POPResult{quickAt("dead", 0), quickAt("b", 50), quickAt("c", 30)}
	errs := []report.Error{{POPID: "c", Test: "latency", Message: "boom"}}
	assert.Equal(t, []string{"b"}, nettest.SelectFullPOPs(quick, errs, 100, 3))
}

func TestSelectFullPOPsNoneWhenAllLatencyFailed(t *testing.T) {
	quick := []report.POPResult{quickAt("a", 0), quickAt("b", 0)}
	errs := []report.Error{
		{POPID: "a", Test: "latency", Message: "x"},
		{POPID: "b", Test: "latency", Message: "y"},
	}
	assert.Empty(t, nettest.SelectFullPOPs(quick, errs, 100, 3))
}

func TestRunAllQuickForFarPOPs(t *testing.T) {
	srv := quickProbeServer(t)
	udp := streamServer(t, streamNormal)
	host, port := hostPort(t, srv.Listener.Addr().String())
	cfg := &config.Config{PerTestTimeoutSeconds: 3, UDPProbeMagic: "PFLT", Run: tinyRunSettings()}
	// A tiny threshold leaves only the nearest POP qualifying.
	cfg.Run.FullTestMaxLatencyMS = 0.000001
	var ins []nettest.POPInput
	for _, id := range []string{"a", "b"} {
		ins = append(ins, nettest.POPInput{
			POP:     config.POP{ID: id, RegionLabel: id, Hostname: host, UDPEcho: true},
			BaseURL: srv.URL, TCPPort: port, UDPPort: udp.port,
			DCVTCPPort: 1, DCVUDPPort: udp.port, ForceTCP: true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	progress := make(chan nettest.Progress, 512)
	pops, errs := nettest.RunPOPs(ctx, cfg, ins, progress)
	rows := collectProgress(progress)

	require.Len(t, pops, 2)
	assert.Equal(t, []string{"a", "b"}, []string{pops[0].ID, pops[1].ID})
	depths := []string{pops[0].Depth, pops[1].Depth}
	assert.ElementsMatch(t, []string{report.DepthFull, report.DepthQuick}, depths)
	for _, e := range errs {
		assert.NotEqual(t, "latency", e.Test)
	}

	var far string
	for _, p := range pops {
		if p.Depth == report.DepthQuick {
			far = p.ID
			assert.Nil(t, p.Throughput)
		}
	}
	var quickOnly int
	for _, r := range rows {
		if r.POPID == far && r.Test == "quick" && r.Phase == "done" && strings.HasSuffix(r.Summary, " ms · quick check only") {
			quickOnly++
		}
		if r.POPID == far {
			assert.Contains(t, []string{"quick"}, r.Test)
		}
	}
	assert.Equal(t, 1, quickOnly)
}

func TestRunAllWithAllPOPsUnreachable(t *testing.T) {
	cfg := &config.Config{
		PerTestTimeoutSeconds: 1,
		UDPProbeMagic:         "PFLT",
		Run:                   tinyRunSettings(),
		POPs: []config.POP{
			{ID: "x", Hostname: "127.0.0.1"},
			{ID: "y", Hostname: "127.0.0.1"},
		},
	}
	cfg.Run.DCVTCPPort = 1
	cfg.Run.DCVUDPPort = 1
	var ins []nettest.POPInput
	for _, p := range cfg.POPs {
		ins = append(ins, nettest.POPInput{
			POP: p, BaseURL: "http://127.0.0.1:1", TCPPort: 1, UDPPort: 1,
			DCVTCPPort: 1, DCVUDPPort: 1, ForceTCP: true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pops, errs := nettest.RunPOPs(ctx, cfg, ins, make(chan nettest.Progress, 64))

	require.Len(t, pops, 2)
	for _, p := range pops {
		assert.Equal(t, report.DepthQuick, p.Depth)
	}
	assert.NotEmpty(t, errs)
}
