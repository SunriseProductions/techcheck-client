package wizard

import (
	"testing"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/upload"
)

func TestLastUploadInfoIncludesShareURL(t *testing.T) {
	a := &App{lastUpload: &upload.Response{
		ID:         "r1",
		ReceivedAt: "2026-06-01T10:00:00Z",
		ShareURL:   "https://telemetry.example/r/tok/",
	}}
	info := a.LastUploadInfo()
	if got := info["share_url"]; got != "https://telemetry.example/r/tok/" {
		t.Errorf("share_url = %q; want the share url", got)
	}
}

func TestLastUploadInfoShareURLEmptyBeforeUpload(t *testing.T) {
	a := &App{}
	info := a.LastUploadInfo()
	if _, ok := info["share_url"]; !ok {
		t.Error("share_url key should always be present")
	}
	if got := info["share_url"]; got != "" {
		t.Errorf("share_url should be empty before any upload; got %q", got)
	}
}
