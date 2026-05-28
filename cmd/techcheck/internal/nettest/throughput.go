package nettest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// busyError signals the POP returned 503 with a Retry-After hint. Callers
// (MeasureDownload / MeasureUpload) sleep for retryAfter and try again,
// without counting the wait toward the measurement.
type busyError struct {
	retryAfter time.Duration
}

func (e *busyError) Error() string {
	return fmt.Sprintf("pop busy (retry after %s)", e.retryAfter)
}

// parseRetryAfter reads the Retry-After header in seconds form. Missing or
// unparseable values fall back to a sensible default so we still back off.
func parseRetryAfter(h string, fallback time.Duration) time.Duration {
	if h == "" {
		return fallback
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return fallback
}

// waitForRetry sleeps for d but respects ctx cancellation.
func waitForRetry(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

const bwMaxAttempts = 4

// MeasureDownload fetches url with a GET and returns the effective download
// throughput in Mbps. Mbps is computed as (bytes * 8) / (elapsed_seconds *
// 1_000_000) — decimal megabits, the convention used by networking vendors.
//
// If the POP replies 503 with Retry-After (bandwidth semaphore full), the
// wait is excluded from the measurement — a fresh `start` is taken on each
// attempt and only the successful attempt's transfer time counts.
//
// On context-deadline / partial-read failures the error is wrapped with what
// was actually received so the UI can show "download stalled at 3.2 Mbps
// after 15s (received 6.0 MB of 10 MB)" instead of a bare
// "context deadline exceeded".
func MeasureDownload(ctx context.Context, url string) (float64, error) {
	var lastErr error
	for attempt := 0; attempt < bwMaxAttempts; attempt++ {
		mbps, err := measureDownloadOnce(ctx, url)
		if err == nil {
			return mbps, nil
		}
		var be *busyError
		if !errors.As(err, &be) {
			return 0, err
		}
		lastErr = err
		if waitErr := waitForRetry(ctx, be.retryAfter); waitErr != nil {
			return 0, waitErr
		}
	}
	return 0, fmt.Errorf("download still busy after %d attempts: %w", bwMaxAttempts, lastErr)
}

func measureDownloadOnce(ctx context.Context, url string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return 0, &busyError{retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), 5*time.Second)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("download status %d", resp.StatusCode)
	}
	expected := resp.ContentLength // -1 if unknown
	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)
	if err != nil {
		if n > 0 && elapsed > 0 {
			partialMbps := float64(n*8) / (elapsed.Seconds() * 1_000_000)
			if expected > 0 {
				return 0, fmt.Errorf(
					"download stalled at %.1f Mbps after %s (received %s of %s): %w",
					partialMbps, elapsed.Round(100*time.Millisecond),
					humanBytes(n), humanBytes(expected), err,
				)
			}
			return 0, fmt.Errorf(
				"download stalled at %.1f Mbps after %s (received %s): %w",
				partialMbps, elapsed.Round(100*time.Millisecond), humanBytes(n), err,
			)
		}
		return 0, err
	}
	if elapsed <= 0 {
		return 0, fmt.Errorf("zero elapsed time")
	}
	return float64(n*8) / (elapsed.Seconds() * 1_000_000), nil
}

// humanBytes returns "6.0 MB" / "842 kB" style strings. Decimal (SI) units
// to match the Mbps calculation.
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for rem := n / unit; rem >= unit; rem /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

// MeasureUpload POSTs sizeBytes random bytes to url and returns the effective
// upload throughput in Mbps. Payload is cryptographically random to defeat
// any transparent compression.
//
// 503 Retry-After is honoured and retry wait is excluded from the
// measurement (see MeasureDownload for the pattern).
func MeasureUpload(ctx context.Context, url string, sizeBytes int) (float64, error) {
	buf := make([]byte, sizeBytes)
	if _, err := rand.Read(buf); err != nil {
		return 0, err
	}
	var lastErr error
	for attempt := 0; attempt < bwMaxAttempts; attempt++ {
		mbps, err := measureUploadOnce(ctx, url, buf)
		if err == nil {
			return mbps, nil
		}
		var be *busyError
		if !errors.As(err, &be) {
			return 0, err
		}
		lastErr = err
		if waitErr := waitForRetry(ctx, be.retryAfter); waitErr != nil {
			return 0, waitErr
		}
	}
	return 0, fmt.Errorf("upload still busy after %d attempts: %w", bwMaxAttempts, lastErr)
}

func measureUploadOnce(ctx context.Context, url string, buf []byte) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(buf))

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return 0, &busyError{retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), 5*time.Second)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("upload status %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	if elapsed <= 0 {
		return 0, fmt.Errorf("zero elapsed time")
	}
	return float64(len(buf)*8) / (elapsed.Seconds() * 1_000_000), nil
}
