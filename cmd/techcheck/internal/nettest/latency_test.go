package nettest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestLatencyTCPFallbackAgainstLocalhost(t *testing.T) {
	ln := listenTCP(t)
	defer ln.Close()
	go acceptLoop(ln)

	_, port := hostPort(t, ln.Addr().String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, method, dropped, err := nettest.MeasureLatency(ctx, nettest.LatencyInput{
		Host:        "127.0.0.1",
		TCPPort:     port,
		Samples:     5,
		ForceTCP:    true,
		PerSampleTO: time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, report.LatencyMethodTCP, method)
	assert.Equal(t, report.LatencyMethodTCP, res.Method)
	assert.Equal(t, 0, dropped)
	assert.Greater(t, res.MedianMS, 0.0)
	assert.LessOrEqual(t, res.MinMS, res.MedianMS)
	assert.LessOrEqual(t, res.MedianMS, res.MaxMS)
}

func TestLatencyTCPCountsDroppedSamples(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Port 1 is unbound on most systems; connect attempts fail fast.
	_, _, dropped, err := nettest.MeasureLatency(ctx, nettest.LatencyInput{
		Host: "127.0.0.1", TCPPort: 1, Samples: 3, ForceTCP: true,
		PerSampleTO: 200 * time.Millisecond,
	})
	assert.Error(t, err) // all dropped → error returned
	assert.Equal(t, 3, dropped)
}

func TestLatencyAggregationPercentiles(t *testing.T) {
	// Drive aggregation directly to lock percentile semantics.
	res := nettest.AggregateSamples(report.LatencyMethodTCP, []float64{
		10, 20, 30, 40, 50, 60, 70, 80, 90, 100,
	})
	assert.Equal(t, report.LatencyMethodTCP, res.Method)
	assert.Equal(t, 10.0, res.MinMS)
	assert.Equal(t, 100.0, res.MaxMS)
	// Median of 10 samples: 5th value (index 4) under nearest-rank.
	assert.Equal(t, 50.0, res.MedianMS)
	// p95 of 10 samples: 10th value (index 9) under nearest-rank.
	assert.Equal(t, 100.0, res.P95MS)
	assert.Greater(t, res.StddevMS, 0.0)
}
