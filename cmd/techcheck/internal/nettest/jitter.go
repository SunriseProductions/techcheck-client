package nettest

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

type JitterInput struct {
	Host         string
	TCPPort      int
	DownloadURL  string
	Duration     time.Duration // defaults to 30s
	PingInterval time.Duration // defaults to 500ms
}

// MeasureJitter runs a background HTTP download while TCP-pinging Host:Port
// at PingInterval for Duration. The variance of the ping RTTs is returned —
// this is the "jitter under load" measurement from PRD §5.3 that catches
// flaky home Wi-Fi that single-shot pings miss. Also returns the count of
// pings that failed to dial during the window; the per-POP runner rolls this
// up into the POPResult.Loss.Pct field.
func MeasureJitter(ctx context.Context, in JitterInput) (report.Jitter, int, error) {
	if in.Duration <= 0 {
		in.Duration = 30 * time.Second
	}
	if in.PingInterval <= 0 {
		in.PingInterval = 500 * time.Millisecond
	}

	// Run the download concurrently for slightly longer than the ping window
	// so the link is saturated for the whole ping phase.
	loadCtx, cancelLoad := context.WithTimeout(ctx, in.Duration+2*time.Second)
	defer cancelLoad()

	loadDone := make(chan struct{})
	go func() {
		defer close(loadDone)
		// Result discarded — we want the side effect of saturating the link.
		_, _ = MeasureDownload(loadCtx, in.DownloadURL)
	}()

	pingCtx, cancelPing := context.WithTimeout(ctx, in.Duration)
	defer cancelPing()

	addr := net.JoinHostPort(in.Host, fmt.Sprintf("%d", in.TCPPort))
	ticker := time.NewTicker(in.PingInterval)
	defer ticker.Stop()

	var samples []float64
	dropped := 0
	// Clamp dial timeout to PingInterval so a slow dial can't overlap the
	// next tick; 500ms is the hard ceiling for a well-behaved link.
	dialTimeout := in.PingInterval
	if dialTimeout > 500*time.Millisecond {
		dialTimeout = 500 * time.Millisecond
	}
	d := net.Dialer{Timeout: dialTimeout}

	for {
		select {
		case <-pingCtx.Done():
			<-loadDone // let the load finish before returning
			return computeVariance(samples), dropped, nil
		case <-ticker.C:
			start := time.Now()
			conn, err := d.DialContext(pingCtx, "tcp", addr)
			if err != nil {
				dropped++
				continue
			}
			_ = conn.Close()
			samples = append(samples, float64(time.Since(start).Microseconds())/1000.0)
		}
	}
}

// computeVariance returns sample variance in ms² — this is what the report
// schema calls VarianceMS (naming is per PRD §7.4). Fewer than 2 samples
// yields zero by convention.
func computeVariance(samples []float64) report.Jitter {
	if len(samples) < 2 {
		return report.Jitter{VarianceMS: 0}
	}
	var sum float64
	for _, v := range samples {
		sum += v
	}
	mean := sum / float64(len(samples))
	var sq float64
	for _, v := range samples {
		sq += (v - mean) * (v - mean)
	}
	return report.Jitter{VarianceMS: sq / float64(len(samples))}
}
