package nettest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// LookupClientGeo queries ipinfo.io for city-level geolocation of the
// caller's current public IP. No API key required for anonymous requests;
// rate-limited to 50k/month per IP. Returns a zero-value ClientGeo and an
// error on any failure; callers should treat that as "unknown" not fatal.
func LookupClientGeo(ctx context.Context) (report.ClientGeo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipinfo.io/json", nil)
	if err != nil {
		return report.ClientGeo{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return report.ClientGeo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return report.ClientGeo{}, fmt.Errorf("ipinfo.io status %d", resp.StatusCode)
	}

	var body struct {
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
		Loc     string `json:"loc"` // "lat,lon"
		Org     string `json:"org"` // "AS12345 Provider Name"
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return report.ClientGeo{}, err
	}

	geo := report.ClientGeo{
		City:    body.City,
		Region:  body.Region,
		Country: body.Country,
		ISP:     cleanOrg(body.Org),
	}
	if lat, lon, ok := parseLoc(body.Loc); ok {
		geo.Lat = lat
		geo.Lon = lon
	}
	return geo, nil
}

func parseLoc(s string) (float64, float64, bool) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// cleanOrg strips the leading AS number (e.g. "AS12345 ") from ipinfo.io's
// "org" field, leaving just the provider name.
func cleanOrg(org string) string {
	org = strings.TrimSpace(org)
	if strings.HasPrefix(org, "AS") {
		if idx := strings.Index(org, " "); idx > 0 {
			return strings.TrimSpace(org[idx+1:])
		}
	}
	return org
}
