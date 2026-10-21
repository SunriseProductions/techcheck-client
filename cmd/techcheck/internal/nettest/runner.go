package nettest

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// Progress is emitted to the progress channel as each per-POP test starts,
// completes, or fails. The wizard frontend forwards these events to the UI.
type Progress struct {
	POPID   string
	Test    string
	Phase   string // "start" | "done" | "failed"
	Err     string
	Summary string // human-friendly value on done, e.g. "42 ms", "102 Mbps"
}

// POPInput overrides derivation in tests. In production the runner uses the
// POP's Hostname with scheme "https", ports 443 (TCP) / 4172 (UDP), and the
// DCV ports from cfg.Run.
type POPInput struct {
	POP        config.POP
	BaseURL    string         // test override; if empty, derived from POP.Hostname
	TCPPort    int            // test override; defaults to 443
	UDPPort    int            // test override; defaults to 4172
	DCVTCPPort int            // test override; defaults to cfg.Run.DCVTCPPort
	DCVUDPPort int            // test override; defaults to cfg.Run.DCVUDPPort
	TLSRoots   *x509.CertPool // test override; nil uses the system roots
	ForceTCP   bool           // test override; skip ICMP attempt
	ClockSkew  bool           // FullPOP measures clock skew only when set
}

// popSetup holds the values QuickPOP and FullPOP both derive from their input.
type popSetup struct {
	host       string
	baseURL    string
	tcpPort    int
	udpPort    int
	dcvTCPPort int
	dcvUDPPort int
	timeout    time.Duration
	run        config.RunSettings
}

func newPOPSetup(in POPInput, cfg *config.Config) popSetup {
	s := popSetup{
		host: in.POP.Hostname, baseURL: in.BaseURL,
		tcpPort: in.TCPPort, udpPort: in.UDPPort,
		dcvTCPPort: in.DCVTCPPort, dcvUDPPort: in.DCVUDPPort,
		run: cfg.Run.WithDefaults(),
	}
	if s.baseURL == "" {
		s.baseURL = fmt.Sprintf("https://%s", in.POP.Hostname)
	}
	if s.tcpPort == 0 {
		s.tcpPort = 443
	}
	if s.udpPort == 0 {
		s.udpPort = 4172
	}
	if s.dcvTCPPort == 0 {
		s.dcvTCPPort = s.run.DCVTCPPort
	}
	if s.dcvUDPPort == 0 {
		s.dcvUDPPort = s.run.DCVUDPPort
	}
	s.timeout = time.Duration(cfg.PerTestTimeoutSeconds) * time.Second
	if s.timeout <= 0 {
		s.timeout = 15 * time.Second
	}
	return s
}

// RunPOP executes every network test for one POP: the quick phase, then the
// full phase with clock skew.
func RunPOP(ctx context.Context, in POPInput, cfg *config.Config, progress chan<- Progress) (report.POPResult, []report.Error) {
	in.ClockSkew = true
	quick, qerrs := QuickPOP(ctx, in, cfg, progress)
	res, ferrs := FullPOP(ctx, in, cfg, quick, progress)
	return res, append(qerrs, ferrs...)
}

// QuickPOP is phase 1: a cheap check of one POP. It measures latency (10
// samples), the TCP 443 and 8443 ports, the UDP 4172 and 8443 echoes, and
// DNS. The only progress rows are "quick" start and done.
func QuickPOP(ctx context.Context, in POPInput, cfg *config.Config, progress chan<- Progress) (report.POPResult, []report.Error) {
	s := newPOPSetup(in, cfg)
	res := report.POPResult{ID: in.POP.ID, RegionLabel: in.POP.RegionLabel, Depth: report.DepthQuick}
	var errs []report.Error

	emit(progress, in.POP.ID, "quick", "start", "")
	latCtx, cancel := context.WithTimeout(ctx, s.timeout)
	lat, latMethod, _, _, err := MeasureLatency(latCtx, LatencyInput{
		Host: s.host, TCPPort: s.tcpPort, Samples: 10, ForceTCP: in.ForceTCP,
	})
	cancel()
	if err != nil {
		// Record the attempted method so the zero-value result is still
		// schema-valid (method must be "icmp" or "tcp", not empty).
		res.Latency = report.LatencyResult{Method: latMethod}
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "latency", Message: err.Error()})
		emit(progress, in.POP.ID, "quick", "failed", err.Error())
	} else {
		res.Latency = lat
		emitSummary(progress, in.POP.ID, "quick", "done", "", fmt.Sprintf("%.0f ms", lat.MedianMS))
	}

	// TCP 443.
	res.TCP443 = MeasureTCP(ctx, s.host, s.tcpPort, s.timeout)

	// TCP 8443. A blocked port is a result, not an error.
	tlsRes := MeasureTLS(ctx, TLSCheckInput{
		Host: s.host, Port: s.dcvTCPPort, ServerName: in.POP.Hostname,
		ExpectedRoots: s.run.TLSExpectedRoots, Roots: in.TLSRoots, Timeout: s.timeout,
	})
	res.TCP8443 = &tlsRes

	// UDP 4172 and 8443.
	res.UDP4172 = report.Reachability{Status: report.ReachabilityBlocked}
	udp8443 := report.Reachability{Status: report.ReachabilityBlocked}
	if in.POP.UDPEcho {
		udCtx, udCancel := context.WithTimeout(ctx, s.timeout)
		udp, err := MeasureUDPEcho(udCtx, UDPEchoInput{
			Host: s.host, Port: s.udpPort, Magic: cfg.UDPProbeMagic, Timeout: s.timeout,
		})
		udCancel()
		res.UDP4172 = udp
		if err != nil {
			errs = append(errs, report.Error{POPID: in.POP.ID, Test: "udp_4172", Message: err.Error()})
		}

		// Port 8443 blocked is a result: old beacons do not listen there.
		udCtx, udCancel = context.WithTimeout(ctx, s.timeout)
		udp8443, _ = MeasureUDPEcho(udCtx, UDPEchoInput{
			Host: s.host, Port: s.dcvUDPPort, Magic: cfg.UDPProbeMagic, Timeout: s.timeout,
		})
		udCancel()
	}
	res.UDP8443 = &udp8443

	// DNS.
	if ms, err := MeasureDNS(ctx, in.POP.Hostname); err == nil {
		res.DNSMS = ms
	} else {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "dns", Message: err.Error()})
	}

	return res, errs
}

// FullPOP is phase 2: the full tests for one POP, starting from its quick
// result. A field is set only when its measurement worked. A failed test
// leaves the field nil and adds an errors entry.
//
// Sequencing rationale: latency + throughput + jitter are ordered because
// jitter specifically measures RTT under the throughput-induced load.
func FullPOP(ctx context.Context, in POPInput, cfg *config.Config, quick report.POPResult, progress chan<- Progress) (report.POPResult, []report.Error) {
	s := newPOPSetup(in, cfg)
	res := quick
	res.Depth = report.DepthFull
	var errs []report.Error
	timeout := s.timeout

	// The quick phase already measured these ports. Show them as rows.
	if res.TCP8443 != nil {
		summary := string(res.TCP8443.Status)
		if res.TCP8443.TLSIntercepted {
			summary += " (TLS intercepted)"
		}
		emitSummary(progress, in.POP.ID, "tcp_8443", "done", "", summary)
	}
	if res.UDP8443 != nil {
		emitSummary(progress, in.POP.ID, "udp_8443", "done", "", string(res.UDP8443.Status))
	}

	// Accumulate attempt + drop counts from latency + jitter to populate
	// Loss.Pct from the real number of pings made, not a theoretical estimate.
	totalAttempts := 0
	totalDropped := 0

	// Latency (20 samples).
	emit(progress, in.POP.ID, "latency", "start", "")
	latCtx, cancel := context.WithTimeout(ctx, timeout)
	lat, latMethod, latAttempts, latDropped, err := MeasureLatency(latCtx, LatencyInput{
		Host: s.host, TCPPort: s.tcpPort, Samples: 20, ForceTCP: in.ForceTCP,
	})
	cancel()
	if err != nil {
		if res.Latency.Method == "" {
			res.Latency = report.LatencyResult{Method: latMethod}
		}
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "latency", Message: err.Error()})
		emit(progress, in.POP.ID, "latency", "failed", err.Error())
	} else {
		res.Latency = lat
		emitSummary(progress, in.POP.ID, "latency", "done", "",
			fmt.Sprintf("%.0f ms (%s)", lat.MedianMS, latMethod))
	}
	totalAttempts += latAttempts
	totalDropped += latDropped

	// Throughput. Set only when both directions were measured.
	var tp report.Throughput
	downOK, upOK := false, false
	emit(progress, in.POP.ID, "throughput_down", "start", "")
	dctx, dcancel := context.WithTimeout(ctx, timeout)
	if mbps, err := MeasureDownload(dctx, s.baseURL+"/preflight/download/10mb"); err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "throughput_down", Message: err.Error()})
		emit(progress, in.POP.ID, "throughput_down", "failed", err.Error())
	} else {
		tp.DownMbps = mbps
		downOK = true
		emitSummary(progress, in.POP.ID, "throughput_down", "done", "",
			fmt.Sprintf("%.1f Mbps", mbps))
	}
	dcancel()

	emit(progress, in.POP.ID, "throughput_up", "start", "")
	uctx, ucancel := context.WithTimeout(ctx, timeout)
	if mbps, err := MeasureUpload(uctx, s.baseURL+"/preflight/upload", 5*1024*1024); err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "throughput_up", Message: err.Error()})
		emit(progress, in.POP.ID, "throughput_up", "failed", err.Error())
	} else {
		tp.UpMbps = mbps
		upOK = true
		emitSummary(progress, in.POP.ID, "throughput_up", "done", "",
			fmt.Sprintf("%.1f Mbps", mbps))
	}
	ucancel()
	if downOK && upOK {
		res.Throughput = &tp
	}

	// Jitter under load. Gets its own larger budget — the jitter window is
	// itself up to 30s per PRD §5.3 and we add ~2s of slack for the load.
	emit(progress, in.POP.ID, "jitter", "start", "")
	jitterDuration := 30 * time.Second
	if timeout < jitterDuration+2*time.Second {
		// Tests pass a short timeout; scale the jitter window down so we
		// still exercise the code path without exceeding the overall budget.
		jitterDuration = timeout - 2*time.Second
		if jitterDuration < 500*time.Millisecond {
			jitterDuration = 500 * time.Millisecond
		}
	}
	jctx, jcancel := context.WithTimeout(ctx, jitterDuration+5*time.Second)
	jit, jAttempts, jDropped, err := MeasureJitter(jctx, JitterInput{
		Host: s.host, TCPPort: s.tcpPort,
		DownloadURL:  s.baseURL + "/preflight/download/10mb",
		Duration:     jitterDuration,
		PingInterval: 500 * time.Millisecond,
	})
	jcancel()
	if err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "jitter", Message: err.Error()})
		emit(progress, in.POP.ID, "jitter", "failed", err.Error())
	} else {
		res.Jitter = &jit
		emitSummary(progress, in.POP.ID, "jitter", "done", "",
			JitterSummary(jit))
	}
	totalAttempts += jAttempts
	totalDropped += jDropped

	// Loss percentage — latency + jitter attempts combined. Unset when no
	// ping was made.
	if totalAttempts > 0 {
		res.Loss = &report.Loss{Pct: float64(totalDropped) / float64(totalAttempts) * 100}
	}

	// MTU.
	emit(progress, in.POP.ID, "mtu", "start", "")
	mctx, mcancel := context.WithTimeout(ctx, timeout)
	mtu, mtuErr := MeasureMTU(mctx, s.baseURL+"/preflight/mtu")
	mcancel()
	if mtuErr != nil {
		// MeasureMTU returns black-hole on transport errors; for request-
		// construction errors the status is empty — default to black-hole.
		if mtu.Status == "" {
			mtu.Status = report.MTUStatusBlackHole
		}
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "mtu", Message: mtuErr.Error()})
		emit(progress, in.POP.ID, "mtu", "failed", mtuErr.Error())
	} else {
		emitSummary(progress, in.POP.ID, "mtu", "done", "", mtu.Status)
	}
	res.MTU = &mtu

	// UDP streams. Each runs only when its port answered the echo probe in
	// the quick phase; a blocked dot already says why a stream is missing.
	var streams report.UDPStream
	stream := s.run.UDPStream
	if quick.UDP4172.Status == report.ReachabilityReachable {
		emit(progress, in.POP.ID, "udp_stream_4172", "start", "")
		sctx, scancel := context.WithTimeout(ctx, streamBudget(stream.TeradiciSeconds, timeout))
		st, anyData, err := MeasureUDPStream(sctx, StreamInput{
			Host: s.host, Port: s.udpPort, Magic: cfg.UDPProbeMagic,
			RateMbps: stream.TeradiciMbps, PacketBytes: stream.PacketBytes,
			Duration: secondsDuration(stream.TeradiciSeconds), Timeout: timeout,
		})
		scancel()
		if err != nil {
			errs = append(errs, report.Error{POPID: in.POP.ID, Test: "udp_stream_4172", Message: err.Error()})
			emit(progress, in.POP.ID, "udp_stream_4172", "failed", err.Error())
		} else {
			large := report.LargePacketsOK
			if !anyData {
				large = report.LargePacketsDropped
			}
			streams.Teradici = &report.StreamRun{Port: s.udpPort, StreamStep: st, LargePackets: large}
			emitSummary(progress, in.POP.ID, "udp_stream_4172", "done", "",
				fmt.Sprintf("%.2f %% loss", st.LossPct))
		}
	}
	if quick.UDP8443 != nil && quick.UDP8443.Status == report.ReachabilityReachable {
		emit(progress, in.POP.ID, "udp_stream_8443", "start", "")
		step := secondsDuration(stream.StepSeconds)
		sctx, scancel := context.WithTimeout(ctx,
			time.Duration(len(stream.LadderMbps))*streamBudget(stream.StepSeconds, timeout))
		ladder, err := RunLadder(sctx, StreamInput{
			Host: s.host, Port: s.dcvUDPPort, Magic: cfg.UDPProbeMagic,
			PacketBytes: stream.PacketBytes, Timeout: timeout,
		}, stream.LadderMbps, step, stream.LossLimitPct)
		scancel()
		if err != nil {
			errs = append(errs, report.Error{POPID: in.POP.ID, Test: "udp_stream_8443", Message: err.Error()})
			emit(progress, in.POP.ID, "udp_stream_8443", "failed", err.Error())
		} else {
			emitSummary(progress, in.POP.ID, "udp_stream_8443", "done", "", ladderSummary(ladder))
		}
		if len(ladder.Steps) > 0 {
			streams.DCV = &ladder
		}
	}
	if streams.Teradici != nil || streams.DCV != nil {
		res.UDPStream = &streams
	}

	// Clock skew.
	if in.ClockSkew {
		emit(progress, in.POP.ID, "clock_skew", "start", "")
		cctx, ccancel := context.WithTimeout(ctx, timeout)
		if skew, err := MeasureClockSkew(cctx, s.baseURL+"/preflight/echo-ts"); err != nil {
			errs = append(errs, report.Error{POPID: in.POP.ID, Test: "clock_skew", Message: err.Error()})
			emit(progress, in.POP.ID, "clock_skew", "failed", err.Error())
		} else {
			res.ClockSkewMS = &skew
			emitSummary(progress, in.POP.ID, "clock_skew", "done", "",
				fmt.Sprintf("%d ms", skew))
		}
		ccancel()
	}

	return res, errs
}

func secondsDuration(sec float64) time.Duration {
	return time.Duration(sec * float64(time.Second))
}

// streamBudget bounds one stream run: its duration, the handshake and busy
// retries (each waits up to the per-test timeout), and the end-of-stream wait.
func streamBudget(sec float64, timeout time.Duration) time.Duration {
	return secondsDuration(sec) + 4*timeout + 10*time.Second
}

// ladderSummary reports the clean rate and the loss of the last step run.
func ladderSummary(l report.StreamLadder) string {
	loss := 0.0
	if n := len(l.Steps); n > 0 {
		loss = l.Steps[n-1].LossPct
	}
	return fmt.Sprintf("%g Mbps clean, %.2f %% loss", l.MaxCleanMbps, loss)
}

// RunAll fans RunPOP across cfg.POPs with bounded concurrency. It also
// fetches /preflight/whoami once (from the first POP) to populate the
// top-level public_ip / detected_geo fields before the per-POP loop starts.
func RunAll(ctx context.Context, cfg *config.Config, progress chan<- Progress) (report.Network, []report.Error) {
	net := report.Network{POPs: []report.POPResult{}}

	if len(cfg.POPs) > 0 {
		whoamiURL := fmt.Sprintf("https://%s/preflight/whoami", cfg.POPs[0].Hostname)
		if w, err := Whoami(ctx, whoamiURL); err == nil {
			net.PublicIP = w.PublicIP
			net.DetectedGeo = w.Geo
		}
	}

	// Client-side geolocation via ipinfo.io. Best-effort; a lookup failure
	// leaves ClientGeo as zero values rather than blocking the run.
	if geo, err := LookupClientGeo(ctx); err == nil {
		net.ClientGeo = geo
	}

	// Run POPs sequentially. Parallel POP tests saturate consumer broadband
	// links, causing UDP echo replies to drop and downstream throughput to
	// time out. Sequential is slower but gives reliable numbers.
	var allErrs []report.Error
	for _, pop := range cfg.POPs {
		res, errs := RunPOP(ctx, POPInput{POP: pop}, cfg, progress)
		net.POPs = append(net.POPs, res)
		allErrs = append(allErrs, errs...)
	}
	return net, allErrs
}

// emit sends a Progress event without blocking. If the channel is full or nil
// the event is dropped — progress UI is best-effort.
func emit(ch chan<- Progress, popID, test, phase, errMsg string) {
	emitSummary(ch, popID, test, phase, errMsg, "")
}

func emitSummary(ch chan<- Progress, popID, test, phase, errMsg, summary string) {
	if ch == nil {
		return
	}
	select {
	case ch <- Progress{POPID: popID, Test: test, Phase: phase, Err: errMsg, Summary: summary}:
	default:
	}
}
