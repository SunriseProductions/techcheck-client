package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

type Options struct {
	URL        string
	Token      string
	Timeout    time.Duration // default 30s
	MaxRetries int           // default 0
}

type Response struct {
	ID         string `json:"id"`
	ReceivedAt string `json:"received_at"`
}

// Upload POSTs the report with exponential backoff retries on transient
// failure. The server may return 201 for a fresh report or 200 for a
// duplicate run_id (per PRD §7.3 idempotency); both are treated as success
// and the caller receives the server-side record either way.
//
// Retries fire on 5xx status and transport errors only. 4xx is terminal —
// retrying a malformed / unauthorised request won't help.
func Upload(ctx context.Context, opt Options, r *report.Report) (*Response, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}

	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: opt.Timeout}
	backoff := 500 * time.Millisecond
	var lastErr error

	for attempt := 0; attempt <= opt.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, doErr := doUpload(ctx, client, opt, body)
		if doErr != nil {
			// Transport / request-construction error — retryable.
			lastErr = doErr
		} else {
			switch {
			case resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK:
				var out Response
				if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
					resp.Body.Close()
					return nil, fmt.Errorf("decode response: %w", err)
				}
				resp.Body.Close()
				return &out, nil
			case resp.StatusCode >= 400 && resp.StatusCode < 500:
				msg, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return nil, fmt.Errorf("server %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
			default:
				// 5xx or other — retryable.
				raw, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("server %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
			}
		}

		if attempt < opt.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	}
	return nil, fmt.Errorf("upload failed after %d attempts: %w", opt.MaxRetries+1, lastErr)
}

func doUpload(ctx context.Context, client *http.Client, opt Options, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opt.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opt.Token)
	return client.Do(req)
}
