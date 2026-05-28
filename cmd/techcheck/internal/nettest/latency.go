package nettest

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// LatencyInput describes one latency measurement run against a target host.
type LatencyInput struct {
	Host        string
	TCPPort     int           // used when ICMP is unavailable; typically 443
	Samples     int           // defaults to 20
	PerSampleTO time.Duration // defaults to 1s
	ForceTCP    bool          // tests: skip ICMP attempt entirely
}

// MeasureLatency runs round-trip samples against Host. It prefers ICMP echo;
// on privilege or availability failure it falls back to TCP connect to
// TCPPort. Returns the aggregated result, the method actually used, and the
// number of dropped samples (out of in.Samples) so upstream can populate
// POPResult.Loss.Pct.
func MeasureLatency(ctx context.Context, in LatencyInput) (report.LatencyResult, string, int, error) {
	if in.Samples <= 0 {
		in.Samples = 20
	}
	if in.PerSampleTO <= 0 {
		in.PerSampleTO = time.Second
	}

	method := report.LatencyMethodICMP
	var samples []float64
	var dropped int

	if !in.ForceTCP {
		// Give ICMP a tight budget so the TCP fallback still has time if
		// ICMP is silently dropped (EC2 security groups typically don't
		// allow ICMP — every ping times out). Without this, a 20-sample
		// ICMP attempt eats the entire outer context before TCP can run.
		icmpCtx, icmpCancel := context.WithTimeout(ctx, 3*time.Second)
		s, d, err := icmpPings(icmpCtx, in.Host, in.Samples, in.PerSampleTO)
		icmpCancel()
		if err == nil && len(s) > 0 {
			samples = s
			dropped = d
		} else {
			method = report.LatencyMethodTCP
		}
	} else {
		method = report.LatencyMethodTCP
	}

	if method == report.LatencyMethodTCP {
		s, d, err := tcpPings(ctx, in.Host, in.TCPPort, in.Samples, in.PerSampleTO)
		if err != nil {
			return report.LatencyResult{}, method, d, err
		}
		samples = s
		dropped = d
	}

	if len(samples) == 0 {
		return report.LatencyResult{}, method, dropped, fmt.Errorf("no samples collected")
	}
	return AggregateSamples(method, samples), method, dropped, nil
}

func tcpPings(ctx context.Context, host string, port, n int, to time.Duration) ([]float64, int, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	samples := make([]float64, 0, n)
	dropped := 0
	d := net.Dialer{Timeout: to}
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return samples, dropped, err
		}
		start := time.Now()
		conn, err := d.DialContext(ctx, "tcp", addr)
		elapsed := time.Since(start)
		if err != nil {
			dropped++
			continue
		}
		_ = conn.Close()
		samples = append(samples, float64(elapsed.Microseconds())/1000.0)
	}
	if len(samples) == 0 {
		return nil, dropped, fmt.Errorf("all TCP pings failed to %s", addr)
	}
	return samples, dropped, nil
}

// AggregateSamples reduces a slice of RTT samples (ms) to min / median / p95 /
// max / stddev. Exposed for tests that want to exercise aggregation in
// isolation.
func AggregateSamples(method string, samples []float64) report.LatencyResult {
	if len(samples) == 0 {
		return report.LatencyResult{Method: method}
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	var sum float64
	for _, v := range samples {
		sum += v
	}
	mean := sum / float64(len(samples))

	var sq float64
	for _, v := range samples {
		sq += (v - mean) * (v - mean)
	}
	// Population stddev (divide by n). Samples are the full distribution we
	// measured, not an estimate of a larger population.
	stddev := math.Sqrt(sq / float64(len(samples)))

	return report.LatencyResult{
		Method:   method,
		MinMS:    sorted[0],
		MedianMS: percentile(sorted, 0.5),
		P95MS:    percentile(sorted, 0.95),
		MaxMS:    sorted[len(sorted)-1],
		StddevMS: stddev,
	}
}

// percentile returns the nearest-rank p-th percentile from a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
