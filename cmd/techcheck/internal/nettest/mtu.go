package nettest

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// MeasureMTU fetches url and checks whether the 1400-byte payload arrived
// intact. A short response suggests fragmentation or a black-hole proxy.
// On any transport error the result is black-hole — the error field on the
// POPResult captures the specific cause separately.
func MeasureMTU(ctx context.Context, url string) (report.MTUResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return report.MTUResult{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return report.MTUResult{Status: report.MTUStatusBlackHole}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return report.MTUResult{Status: report.MTUStatusBlackHole}, nil
	}
	switch {
	case len(body) >= 1400:
		return report.MTUResult{Status: report.MTUStatusPass}, nil
	case len(body) > 0:
		return report.MTUResult{Status: report.MTUStatusFragmented}, nil
	default:
		return report.MTUResult{Status: report.MTUStatusBlackHole}, fmt.Errorf("empty response")
	}
}
