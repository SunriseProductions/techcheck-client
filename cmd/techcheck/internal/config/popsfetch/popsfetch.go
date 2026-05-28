// Package popsfetch retrieves the live POP list from the ingest service.
package popsfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
)

const wantSchema = "pops.v1"

type response struct {
	Schema string       `json:"schema"`
	POPs   []config.POP `json:"pops"`
}

// Fetch retrieves the POP list from url with an overall deadline of timeout.
// Returns the POPs on success. Returns an error (and nil POPs) on any failure:
// timeout, non-2xx status, invalid JSON, or unrecognised schema. Callers should
// fall back to their baked-in list on error.
func Fetch(ctx context.Context, url string, timeout time.Duration) ([]config.POP, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}

	var parsed response
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse pops response: %w", err)
	}
	if parsed.Schema != wantSchema {
		return nil, fmt.Errorf("unrecognised schema %q (want %q)", parsed.Schema, wantSchema)
	}
	return parsed.POPs, nil
}
