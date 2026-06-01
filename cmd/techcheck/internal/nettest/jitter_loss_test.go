package nettest_test

import (
	"context"
	"testing"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
)

// TestMeasureJitter_NoLossOnLosslessLink guards the phantom-drop regression.
// Against a localhost listener that always accepts, real packet loss is
// impossible, so the reported dropped count MUST be 0. Duration is an exact
// multiple of PingInterval so the final ticker tick coincides with the window
// deadline — the boundary that previously made the last dial inherit a ~0ms
// budget, fail with a timeout, and get counted as a dropped packet. That
// produced a constant ~2.2% "loss" (1/46) across every POP regardless of the
// real network.
func TestMeasureJitter_NoLossOnLosslessLink(t *testing.T) {
	ln := listenTCP(t)
	defer ln.Close()
	go acceptLoop(ln)
	host, port := hostPort(t, ln.Addr().String())

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, attempts, dropped, err := nettest.MeasureJitter(ctx, nettest.JitterInput{
			Host:         host,
			TCPPort:      port,
			DownloadURL:  "http://127.0.0.1:1/none", // refused fast; the load goroutine is irrelevant here
			Duration:     500 * time.Millisecond,
			PingInterval: 100 * time.Millisecond,
		})
		cancel()
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if dropped != 0 {
			t.Fatalf("run %d: dropped=%d on a lossless localhost link (phantom-drop regression)", i, dropped)
		}
		if attempts == 0 {
			t.Fatalf("run %d: attempts=0; loss must be computed from real pings", i)
		}
	}
}
