package wizard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/wizard"
)

func TestAppExposesConfigValuesToFrontend(t *testing.T) {
	cfg := &config.Config{
		IngestURL:      "https://ingest.example/api/v1/reports",
		UploadToken:    "t",
		ITContactEmail: "it@sunrise.example",
	}
	app := wizard.NewAppWithConfigAndLivePOPs(cfg, "", 0)
	app.Startup(context.Background())
	assert.Equal(t, "it@sunrise.example", app.ITContactEmail())
	assert.Equal(t, "https://ingest.example/api/v1/reports", app.IngestURL())
}

func TestValidateIdentityRejectsEmptyName(t *testing.T) {
	app := wizard.NewAppWithConfigAndLivePOPs(&config.Config{}, "", 0)
	_, err := app.ValidateIdentity("", "jane@example.com")
	require.Error(t, err)
}

func TestValidateIdentityRejectsMalformedEmail(t *testing.T) {
	app := wizard.NewAppWithConfigAndLivePOPs(&config.Config{}, "", 0)
	_, err := app.ValidateIdentity("Jane Doe", "not-an-email")
	require.Error(t, err)
}

func TestValidateIdentityAcceptsGoodInput(t *testing.T) {
	app := wizard.NewAppWithConfigAndLivePOPs(&config.Config{}, "", 0)
	ok, err := app.ValidateIdentity("Jane Doe", "jane@example.com")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLocalReportPathEmptyBeforeRun(t *testing.T) {
	app := wizard.NewAppWithConfigAndLivePOPs(&config.Config{}, "", 0)
	assert.Empty(t, app.LocalReportPath())
}

func TestSendReportWithoutCurrentErrs(t *testing.T) {
	app := wizard.NewAppWithConfigAndLivePOPs(&config.Config{
		IngestURL: "http://localhost:1", UploadToken: "t",
	}, "", 0)
	app.Startup(context.Background())
	err := app.SendReport()
	require.Error(t, err)
}

func TestNewAppUsesLivePOPsWhenReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/config/pops" {
			_, _ = w.Write([]byte(`{"schema":"pops.v1","pops":[
				{"id":"live-1","region_label":"Live","hostname":"live.beacon.example","udp_echo":true}
			]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &config.Config{
		IngestURL: srv.URL + "/api/v1/reports",
		POPs:      []config.POP{{ID: "baked-1"}},
	}
	app := wizard.NewAppWithConfigAndLivePOPs(cfg, srv.URL+"/api/v1/config/pops", 2*time.Second)
	assert.Len(t, app.POPs(), 1)
	assert.Equal(t, "live-1", app.POPs()[0].ID)
}

func TestNewAppFallsBackWhenFetchFails(t *testing.T) {
	cfg := &config.Config{
		IngestURL: "http://127.0.0.1:1",
		POPs:      []config.POP{{ID: "baked-1"}},
	}
	app := wizard.NewAppWithConfigAndLivePOPs(cfg, "http://127.0.0.1:1/api/v1/config/pops", 100*time.Millisecond)
	assert.Len(t, app.POPs(), 1)
	assert.Equal(t, "baked-1", app.POPs()[0].ID)
}

func TestCheckForUpdate_ReturnsStatusFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/sunrise-techcheck/latest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"app_id":"sunrise-techcheck","latest":"0.2.0","released_at":"2026-04-20T14:00:00Z",
			"mandatory_update":false,"minimum_supported":"0.1.0",
			"is_current_supported":true,"is_update_available":true,
			"artifact":{"platform":"darwin","arch":"arm64","download_url":"https://x","sha256":"a","size_bytes":1},
			"release_notes_md":"","release_notes_html":"<p>hi</p>"
		}`))
	}))
	defer srv.Close()

	cfg := &config.Config{IngestURL: srv.URL + "/api/v1/reports"}
	app := wizard.NewAppWithConfigAndLivePOPs(cfg, "", 0)
	app.Startup(context.Background())

	s := app.CheckForUpdate()
	if !s.UpdateAvailable || s.Latest != "0.2.0" {
		t.Errorf("unexpected status: %+v", s)
	}
	if !s.OK {
		t.Error("OK should be true on success")
	}
}

func TestCheckForUpdate_NetworkErrorReturnsZeroStatus(t *testing.T) {
	cfg := &config.Config{IngestURL: "http://127.0.0.1:1/api/v1/reports"}
	app := wizard.NewAppWithConfigAndLivePOPs(cfg, "", 0)
	app.Startup(context.Background())

	s := app.CheckForUpdate()
	if s.UpdateAvailable || s.IsSupported {
		t.Errorf("zero Status expected; got %+v", s)
	}
	if s.OK {
		t.Error("OK should be false on error")
	}
}

