package nettest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MeasureClockSkew fetches the echo-ts endpoint and returns the signed
// difference (local_clock - server_clock) in milliseconds. A positive result
// means the local clock is ahead of the server. Large skew breaks TLS cert
// validation and Kerberos, so IT uses this to spot drifting home machines.
func MeasureClockSkew(ctx context.Context, url string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("echo-ts status %d", resp.StatusCode)
	}
	local := time.Now().UTC()
	var body struct {
		ISO string `json:"iso"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode echo-ts: %w", err)
	}
	server, err := time.Parse(time.RFC3339Nano, body.ISO)
	if err != nil {
		return 0, fmt.Errorf("parse iso: %w", err)
	}
	return local.Sub(server).Milliseconds(), nil
}
