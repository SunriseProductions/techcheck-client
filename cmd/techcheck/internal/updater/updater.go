// Package updater queries the ingest server for the latest published
// version of a client app and reports whether an update is available or
// the running version has been deprecated.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Status is the result of a single version check. Zero value means "no
// update, not in an unsupported state" — safe for the UI to render as
// "carry on, no banner".
type Status struct {
	OK               bool // True only when Check successfully reached the server and decoded a 2xx response.
	UpdateAvailable  bool
	MandatoryUpdate  bool
	IsSupported      bool
	Latest           string
	ReleasedAt       time.Time
	ReleaseNotesHTML string
	DownloadURL      string
}

type apiResponse struct {
	AppID              string `json:"app_id"`
	Latest             string `json:"latest"`
	ReleasedAt         string `json:"released_at"`
	MandatoryUpdate    bool   `json:"mandatory_update"`
	MinimumSupported   string `json:"minimum_supported"`
	IsCurrentSupported bool   `json:"is_current_supported"`
	IsUpdateAvailable  bool   `json:"is_update_available"`
	Artifact           struct {
		Platform    string `json:"platform"`
		Arch        string `json:"arch"`
		DownloadURL string `json:"download_url"`
		SHA256      string `json:"sha256"`
		SizeBytes   int64  `json:"size_bytes"`
		MinimumOS   string `json:"minimum_os"`
	} `json:"artifact"`
	ReleaseNotesMD   string `json:"release_notes_md"`
	ReleaseNotesHTML string `json:"release_notes_html"`
}

// Check queries ingestBase/api/v1/apps/{appID}/latest using the running
// binary's platform/arch. A zero Status + error is returned on network
// failure or non-2xx responses.
func Check(ctx context.Context, ingestBase, appID, currentVersion string) (Status, error) {
	platform, arch := CurrentPlatform()
	q := url.Values{}
	q.Set("platform", platform)
	q.Set("arch", arch)
	q.Set("current_version", currentVersion)

	u := fmt.Sprintf("%s/api/v1/apps/%s/latest?%s", ingestBase, url.PathEscape(appID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Status{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return Status{}, fmt.Errorf("updater: server returned %d", resp.StatusCode)
	}

	const maxResponseBytes = 1 << 20 // 1 MiB — release notes plus metadata; artifact binaries are NOT in this response.

	var body apiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return Status{}, fmt.Errorf("updater: decode response: %w", err)
	}

	downloadURL := ""
	if u, err := url.Parse(body.Artifact.DownloadURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
		downloadURL = body.Artifact.DownloadURL
	}

	released, _ := time.Parse(time.RFC3339, body.ReleasedAt)
	return Status{
		OK:               true,
		UpdateAvailable:  body.IsUpdateAvailable,
		MandatoryUpdate:  body.MandatoryUpdate,
		IsSupported:      body.IsCurrentSupported,
		Latest:           body.Latest,
		ReleasedAt:       released,
		ReleaseNotesHTML: body.ReleaseNotesHTML,
		DownloadURL:      downloadURL,
	}, nil
}
