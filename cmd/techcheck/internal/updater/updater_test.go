package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fakeServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestCheck_UpdateAvailable(t *testing.T) {
	srv := fakeServer(t, 200, map[string]any{
		"app_id":               "sunrise-techcheck",
		"latest":               "0.2.0",
		"released_at":          "2026-04-20T14:00:00Z",
		"mandatory_update":     false,
		"minimum_supported":    "0.1.0",
		"is_current_supported": true,
		"is_update_available":  true,
		"artifact": map[string]any{
			"platform":     "darwin",
			"arch":         "arm64",
			"download_url": "https://example.com/x.dmg",
			"sha256":       "abc",
			"size_bytes":   1,
		},
		"release_notes_md":   "## hi",
		"release_notes_html": "<h2>hi</h2>",
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := Check(ctx, srv.URL, "sunrise-techcheck", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !s.UpdateAvailable {
		t.Error("UpdateAvailable should be true")
	}
	if s.MandatoryUpdate {
		t.Error("MandatoryUpdate should be false")
	}
	if !s.IsSupported {
		t.Error("IsSupported should be true")
	}
	if s.Latest != "0.2.0" {
		t.Errorf("Latest = %q", s.Latest)
	}
	if s.DownloadURL != "https://example.com/x.dmg" {
		t.Errorf("DownloadURL = %q", s.DownloadURL)
	}
	if s.ReleaseNotesHTML != "<h2>hi</h2>" {
		t.Errorf("ReleaseNotesHTML = %q", s.ReleaseNotesHTML)
	}
	if !s.OK {
		t.Error("OK should be true on success")
	}
}

func TestCheck_NotSupported(t *testing.T) {
	srv := fakeServer(t, 200, map[string]any{
		"app_id":               "sunrise-techcheck",
		"latest":               "0.2.0",
		"released_at":          "2026-04-20T14:00:00Z",
		"mandatory_update":     false,
		"minimum_supported":    "0.2.0",
		"is_current_supported": false,
		"is_update_available":  true,
		"artifact": map[string]any{
			"platform": "darwin", "arch": "arm64",
			"download_url": "https://example.com/x.dmg", "sha256": "x", "size_bytes": 1,
		},
		"release_notes_md":   "",
		"release_notes_html": "",
	})
	defer srv.Close()

	s, err := Check(context.Background(), srv.URL, "sunrise-techcheck", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if s.IsSupported {
		t.Error("IsSupported should be false")
	}
}

func TestCheck_NetworkErrorReturnsZeroStatus(t *testing.T) {
	s, err := Check(context.Background(), "http://127.0.0.1:1", "sunrise-techcheck", "0.1.0")
	if err == nil {
		t.Fatal("expected error")
	}
	if s.UpdateAvailable || s.IsSupported {
		t.Errorf("zero Status expected; got %+v", s)
	}
	if s.OK {
		t.Error("OK should be false on error")
	}
}

func TestCheck_Non200ReturnsError(t *testing.T) {
	srv := fakeServer(t, 500, map[string]any{"error": "boom"})
	defer srv.Close()
	s, err := Check(context.Background(), srv.URL, "sunrise-techcheck", "0.1.0")
	if err == nil {
		t.Fatal("expected error")
	}
	if s.OK {
		t.Error("OK should be false on error")
	}
}

func TestCheck_OversizedBodyIsBounded(t *testing.T) {
	huge := make([]byte, 2<<20) // 2 MiB of spaces — bigger than the cap
	for i := range huge {
		huge[i] = ' '
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Open a partial JSON, stream spaces, never close — the LimitReader
		// will cut off mid-stream and the decoder must return an error quickly.
		w.Write([]byte(`{"app_id":"x","latest":"0.1.0","artifact":{"download_url":"`))
		w.Write(huge)
	}))
	defer srv.Close()

	s, err := Check(context.Background(), srv.URL, "sunrise-techcheck", "0.0.1")
	if err == nil {
		t.Fatal("expected decode error from truncated/oversized response")
	}
	if s.OK {
		t.Error("OK should be false on error")
	}
}

func TestCheck_DropsNonHTTPDownloadURL(t *testing.T) {
	cases := []string{
		"file:///etc/hosts",
		"javascript:alert(1)",
		"",
		"ftp://example.com/x.dmg",
		"not-a-url-at-all",
	}
	for _, badURL := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
                "app_id":"x","latest":"0.1.0","released_at":"2026-04-20T14:00:00Z",
                "mandatory_update":false,"minimum_supported":"",
                "is_current_supported":true,"is_update_available":true,
                "artifact":{"platform":"darwin","arch":"arm64","download_url":%q,"sha256":"a","size_bytes":1},
                "release_notes_md":"","release_notes_html":""
            }`, badURL)
		}))
		s, err := Check(context.Background(), srv.URL, "x", "0.0.1")
		srv.Close()
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", badURL, err)
		}
		if s.DownloadURL != "" {
			t.Errorf("DownloadURL = %q for input %q; want empty", s.DownloadURL, badURL)
		}
	}
}

func TestCheck_KeepsHTTPSDownloadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
            "app_id":"x","latest":"0.1.0","released_at":"2026-04-20T14:00:00Z",
            "mandatory_update":false,"minimum_supported":"",
            "is_current_supported":true,"is_update_available":true,
            "artifact":{"platform":"darwin","arch":"arm64","download_url":"https://ok/x.dmg","sha256":"a","size_bytes":1},
            "release_notes_md":"","release_notes_html":""
        }`))
	}))
	defer srv.Close()
	s, err := Check(context.Background(), srv.URL, "x", "0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if s.DownloadURL != "https://ok/x.dmg" {
		t.Errorf("DownloadURL = %q; want https://ok/x.dmg", s.DownloadURL)
	}
}
