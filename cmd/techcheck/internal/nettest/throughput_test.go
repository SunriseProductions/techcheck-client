package nettest_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
)

func TestDownloadThroughput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(make([]byte, 1<<20)) // 1 MiB
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mbps, err := nettest.MeasureDownload(ctx, srv.URL+"/preflight/download/10mb")
	require.NoError(t, err)
	assert.Greater(t, mbps, 0.0)
}

func TestUploadThroughput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		fmt.Fprintf(w, "%d", n)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mbps, err := nettest.MeasureUpload(ctx, srv.URL+"/preflight/upload", 1<<19) // 512 KiB
	require.NoError(t, err)
	assert.Greater(t, mbps, 0.0)
}

func TestDownloadThroughputNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := nettest.MeasureDownload(context.Background(), srv.URL+"/download")
	require.Error(t, err)
}

// Server replies 503 with Retry-After once, then 200 with payload. Client
// must retry, succeed, and NOT fold the retry wait into the reported rate.
func TestDownloadThroughputRetriesOn503AndExcludesWaitTime(t *testing.T) {
	const retryAfterSec = 1
	const payloadSize = 512 * 1024 // 512 KB — small enough to transfer fast

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", payloadSize))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, payloadSize))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wallStart := time.Now()
	mbps, err := nettest.MeasureDownload(ctx, srv.URL+"/preflight/download/10mb")
	wallElapsed := time.Since(wallStart)
	require.NoError(t, err)

	// Wall clock must include the retry wait.
	assert.GreaterOrEqual(t, wallElapsed, time.Duration(retryAfterSec)*time.Second,
		"wall elapsed %s should include the %ds retry wait", wallElapsed, retryAfterSec)

	// The reported mbps is (512KB * 8) / successful_transfer_seconds. If we
	// had folded the 1s wait in, mbps would be < 4.2. localhost transfer of
	// 512 KB is sub-second, so honest mbps should be comfortably > 10.
	assert.Greater(t, mbps, 10.0,
		"mbps %.1f suggests retry wait was counted toward elapsed time", mbps)

	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "server should have been hit twice")
}

func TestDownloadThroughputReturnsErrorAfterRepeated503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := nettest.MeasureDownload(ctx, srv.URL+"/preflight/download/10mb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still busy")
}

func TestUploadThroughputRetriesOn503(t *testing.T) {
	const retryAfterSec = 1
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mbps, err := nettest.MeasureUpload(ctx, srv.URL+"/preflight/upload", 128*1024)
	require.NoError(t, err)
	assert.Greater(t, mbps, 0.0)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

// A slow server that trickles bytes well beyond the client's context
// deadline. The error must surface what was actually received + effective
// rate, not a bare "context deadline exceeded".
func TestDownloadThroughputDeadlineIncludesPartialRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10485760") // 10 MB claim
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// Write ~100 KB then sit. Client timeout will fire mid-read.
		_, _ = w.Write(make([]byte, 100_000))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, err := nettest.MeasureDownload(ctx, srv.URL+"/download")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Mbps", "error should include effective rate, got: %s", msg)
	assert.Contains(t, msg, "received", "error should name what was received, got: %s", msg)
}
