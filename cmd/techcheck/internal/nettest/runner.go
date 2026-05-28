package nettest

import (
	"context"
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
// POP's Hostname with scheme "https" and ports 443 (TCP) / 4172 (UDP).
type POPInput struct {
	POP      config.POP
	BaseURL  string // test override; if empty, derived from POP.Hostname
	TCPPort  int    // test override; defaults to 443
	UDPPort  int    // test override; defaults to 4172
	ForceTCP bool   // test override; skip ICMP attempt
}

// RunPOP executes every network test for one POP sequentially and returns
// the aggregated POPResult plus any per-test errors. Progress events are
// emitted to the optional channel.
//
// Sequencing rationale: latency + throughput + jitter are ordered because
// jitter specifically measures RTT under the throughput-induced load.
// Reachability and DNS are cheap and run at the end.
func RunPOP(ctx context.Context, in POPInput, cfg *config.Config, progress chan<- Progress) (report.POPResult, []report.Error) {
	baseURL := in.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s", in.POP.Hostname)
	}
	tcpPort := in.TCPPort
	if tcpPort == 0 {
		tcpPort = 443
	}
	udpPort := in.UDPPort
	if udpPort == 0 {
		udpPort = 4172
	}
	host := in.POP.Hostname

	res := report.POPResult{ID: in.POP.ID, RegionLabel: in.POP.RegionLabel}
	var errs []report.Error

	timeout := time.Duration(cfg.PerTestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	// Accumulate drop counts from latency + jitter to populate Loss.Pct.
	totalAttempts := 0
	totalDropped := 0

	// Latency (20 samples).
	emit(progress, in.POP.ID, "latency", "start", "")
	latCtx, cancel := context.WithTimeout(ctx, timeout)
	lat, latMethod, latDropped, err := MeasureLatency(latCtx, LatencyInput{
		Host: host, TCPPort: tcpPort, Samples: 20, ForceTCP: in.ForceTCP,
	})
	cancel()
	if err != nil {
		// Record the attempted method so the zero-value result is still
		// schema-valid (method must be "icmp" or "tcp", not empty).
		res.Latency = report.LatencyResult{Method: latMethod}
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "latency", Message: err.Error()})
		emit(progress, in.POP.ID, "latency", "failed", err.Error())
	} else {
		res.Latency = lat
		emitSummary(progress, in.POP.ID, "latency", "done", "",
			fmt.Sprintf("%.0f ms (%s)", lat.MedianMS, latMethod))
	}
	totalAttempts += 20
	totalDropped += latDropped

	// Download throughput.
	emit(progress, in.POP.ID, "throughput_down", "start", "")
	dctx, dcancel := context.WithTimeout(ctx, timeout)
	if mbps, err := MeasureDownload(dctx, baseURL+"/preflight/download/10mb"); err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "throughput_down", Message: err.Error()})
		emit(progress, in.POP.ID, "throughput_down", "failed", err.Error())
	} else {
		res.Throughput.DownMbps = mbps
		emitSummary(progress, in.POP.ID, "throughput_down", "done", "",
			fmt.Sprintf("%.1f Mbps", mbps))
	}
	dcancel()

	// Upload throughput.
	emit(progress, in.POP.ID, "throughput_up", "start", "")
	uctx, ucancel := context.WithTimeout(ctx, timeout)
	if mbps, err := MeasureUpload(uctx, baseURL+"/preflight/upload", 5*1024*1024); err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "throughput_up", Message: err.Error()})
		emit(progress, in.POP.ID, "throughput_up", "failed", err.Error())
	} else {
		res.Throughput.UpMbps = mbps
		emitSummary(progress, in.POP.ID, "throughput_up", "done", "",
			fmt.Sprintf("%.1f Mbps", mbps))
	}
	ucancel()

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
	jit, jDropped, err := MeasureJitter(jctx, JitterInput{
		Host: host, TCPPort: tcpPort,
		DownloadURL:  baseURL + "/preflight/download/10mb",
		Duration:     jitterDuration,
		PingInterval: 500 * time.Millisecond,
	})
	jcancel()
	if err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "jitter", Message: err.Error()})
		emit(progress, in.POP.ID, "jitter", "failed", err.Error())
	} else {
		res.Jitter = jit
		emitSummary(progress, in.POP.ID, "jitter", "done", "",
			fmt.Sprintf("%.0f ms variance", jit.VarianceMS))
	}
	// Approximate: jitter was roughly `jitterDuration / PingInterval` attempts.
	jitterAttempts := int(jitterDuration / (500 * time.Millisecond))
	if jitterAttempts < 1 {
		jitterAttempts = 1
	}
	totalAttempts += jitterAttempts
	totalDropped += jDropped

	// Loss percentage — latency + jitter attempts combined.
	if totalAttempts > 0 {
		res.Loss.Pct = float64(totalDropped) / float64(totalAttempts) * 100
	}

	// MTU.
	emit(progress, in.POP.ID, "mtu", "start", "")
	mctx, mcancel := context.WithTimeout(ctx, timeout)
	mtu, mtuErr := MeasureMTU(mctx, baseURL+"/preflight/mtu")
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
	res.MTU = mtu

	// Clock skew.
	emit(progress, in.POP.ID, "clock_skew", "start", "")
	cctx, ccancel := context.WithTimeout(ctx, timeout)
	if skew, err := MeasureClockSkew(cctx, baseURL+"/preflight/echo-ts"); err != nil {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "clock_skew", Message: err.Error()})
		emit(progress, in.POP.ID, "clock_skew", "failed", err.Error())
	} else {
		res.ClockSkewMS = skew
		emitSummary(progress, in.POP.ID, "clock_skew", "done", "",
			fmt.Sprintf("%d ms", skew))
	}
	ccancel()

	// UDP 4172.
	if in.POP.UDPEcho {
		emit(progress, in.POP.ID, "udp_4172", "start", "")
		udCtx, udCancel := context.WithTimeout(ctx, timeout)
		udp, err := MeasureUDPEcho(udCtx, UDPEchoInput{
			Host:    host,
			Port:    udpPort,
			Magic:   cfg.UDPProbeMagic,
			Timeout: timeout,
		})
		udCancel()
		res.UDP4172 = udp
		if err != nil {
			errs = append(errs, report.Error{POPID: in.POP.ID, Test: "udp_4172", Message: err.Error()})
			emit(progress, in.POP.ID, "udp_4172", "failed", err.Error())
		} else {
			emitSummary(progress, in.POP.ID, "udp_4172", "done", "", udp.Status)
		}
	} else {
		res.UDP4172 = report.Reachability{Status: report.ReachabilityBlocked}
	}

	// TCP 443.
	emit(progress, in.POP.ID, "tcp_443", "start", "")
	res.TCP443 = MeasureTCP(ctx, host, tcpPort, timeout)
	emitSummary(progress, in.POP.ID, "tcp_443", "done", "", res.TCP443.Status)

	// DNS.
	emit(progress, in.POP.ID, "dns", "start", "")
	if ms, err := MeasureDNS(ctx, in.POP.Hostname); err == nil {
		res.DNSMS = ms
		emitSummary(progress, in.POP.ID, "dns", "done", "",
			fmt.Sprintf("%d ms", ms))
	} else {
		errs = append(errs, report.Error{POPID: in.POP.ID, Test: "dns", Message: err.Error()})
		emit(progress, in.POP.ID, "dns", "failed", err.Error())
	}

	return res, errs
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
