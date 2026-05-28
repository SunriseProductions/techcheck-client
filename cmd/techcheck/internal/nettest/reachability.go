package nettest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// MeasureTCP attempts a single TCP connect within timeout and returns the
// resulting Reachability status. Used for the TCP/443 broker-reachability
// probe per PRD §5.3.
func MeasureTCP(ctx context.Context, host string, port int, timeout time.Duration) report.Reachability {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return report.Reachability{Status: report.ReachabilityBlocked}
	}
	_ = conn.Close()
	return report.Reachability{Status: report.ReachabilityReachable}
}

// MeasureDNS returns the wall-clock milliseconds taken to resolve host via
// the default resolver. Large values point at a broken resolv.conf or an
// overloaded captive DNS server on the user's network.
func MeasureDNS(ctx context.Context, host string) (int64, error) {
	start := time.Now()
	_, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return 0, err
	}
	return time.Since(start).Milliseconds(), nil
}

// WhoamiResult is what the /preflight/whoami endpoint echoes back — the
// client's public IP as seen by the nearest POP, plus the POP's region label.
type WhoamiResult struct {
	PublicIP string
	POPID    string
	Geo      string
}

// Whoami fetches /preflight/whoami and decodes the JSON response.
func Whoami(ctx context.Context, url string) (WhoamiResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return WhoamiResult{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return WhoamiResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WhoamiResult{}, fmt.Errorf("whoami status %d", resp.StatusCode)
	}
	var body struct {
		PublicIP string `json:"public_ip"`
		POPID    string `json:"pop_id"`
		Geo      string `json:"geo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return WhoamiResult{}, err
	}
	return WhoamiResult{PublicIP: body.PublicIP, POPID: body.POPID, Geo: body.Geo}, nil
}
