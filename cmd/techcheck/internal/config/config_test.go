package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/defaults"
)

func TestLoadDefaultsOnly(t *testing.T) {
	// With no ldflags injection and no sidecar, Load returns the non-secret
	// tunables from the embedded defaults; secrets and POP fallback are
	// empty.
	origToken := defaults.UploadToken
	origURL := defaults.IngestURL
	origEmail := defaults.ITContactEmail
	origPOPs := defaults.FallbackPOPsJSON
	t.Cleanup(func() {
		defaults.UploadToken = origToken
		defaults.IngestURL = origURL
		defaults.ITContactEmail = origEmail
		defaults.FallbackPOPsJSON = origPOPs
	})
	defaults.UploadToken = ""
	defaults.IngestURL = ""
	defaults.ITContactEmail = ""
	defaults.FallbackPOPsJSON = ""

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, 15, cfg.PerTestTimeoutSeconds)
	assert.Equal(t, 30, cfg.UploadTimeoutSeconds)
	assert.Equal(t, 2, cfg.UploadRetries)
	assert.False(t, cfg.OmitWifiSSID)
	assert.Equal(t, "PFLT", cfg.UDPProbeMagic)
	assert.Empty(t, cfg.IngestURL)
	assert.Empty(t, cfg.UploadToken)
	assert.Empty(t, cfg.ITContactEmail)
	assert.Empty(t, cfg.POPs)
}

func TestSidecarOverridesDefaults(t *testing.T) {
	// With ldflags injection providing baseline values, a sidecar that sets
	// some fields leaves the others at their ldflags-injected values.
	origToken := defaults.UploadToken
	origURL := defaults.IngestURL
	t.Cleanup(func() {
		defaults.UploadToken = origToken
		defaults.IngestURL = origURL
	})
	defaults.UploadToken = "injected-token"
	defaults.IngestURL = "https://injected.example/reports"

	dir := t.TempDir()
	sidecar := filepath.Join(dir, "preflight.config.json")
	err := os.WriteFile(sidecar, []byte(`{
		"ingest_url": "https://override.example/api/v1/reports",
		"omit_wifi_ssid": true
	}`), 0o644)
	require.NoError(t, err)

	cfg, err := config.Load(sidecar)
	require.NoError(t, err)

	assert.Equal(t, "https://override.example/api/v1/reports", cfg.IngestURL)
	assert.True(t, cfg.OmitWifiSSID)
	assert.Equal(t, "injected-token", cfg.UploadToken, "fields not set by sidecar fall back to ldflags-injected values")
}

func TestLoadRejectsMalformedSidecar(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "preflight.config.json")
	require.NoError(t, os.WriteFile(sidecar, []byte(`{not json`), 0o644))

	_, err := config.Load(sidecar)
	require.Error(t, err)
	assert.ErrorContains(t, err, "parse sidecar")
}

func TestSidecarEmptyPOPsFallsBackToDefaults(t *testing.T) {
	origPOPs := defaults.FallbackPOPsJSON
	t.Cleanup(func() { defaults.FallbackPOPsJSON = origPOPs })
	defaults.FallbackPOPsJSON = `[{"id":"fallback-1","region_label":"Fallback","hostname":"fallback.example.com","udp_echo":true}]`

	dir := t.TempDir()
	sidecar := filepath.Join(dir, "preflight.config.json")
	require.NoError(t, os.WriteFile(sidecar, []byte(`{"pops": []}`), 0o644))

	cfg, err := config.Load(sidecar)
	require.NoError(t, err)
	require.Len(t, cfg.POPs, 1, "empty sidecar pops array must fall back to ldflags FallbackPOPsJSON")
	assert.Equal(t, "fallback-1", cfg.POPs[0].ID)
}

func TestLoadAppliesLDFlagsOverrides(t *testing.T) {
	// Simulate ldflags injection by setting the package vars before Load().
	origToken := defaults.UploadToken
	origURL := defaults.IngestURL
	origEmail := defaults.ITContactEmail
	origPOPs := defaults.FallbackPOPsJSON
	t.Cleanup(func() {
		defaults.UploadToken = origToken
		defaults.IngestURL = origURL
		defaults.ITContactEmail = origEmail
		defaults.FallbackPOPsJSON = origPOPs
	})

	defaults.UploadToken = "ldflag-token"
	defaults.IngestURL = "https://ldflag.example/api/v1/reports"
	defaults.ITContactEmail = "ldflag@example.com"
	defaults.FallbackPOPsJSON = `[{"id":"test-1","region_label":"Test","hostname":"test.example.com","udp_echo":true}]`

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, "ldflag-token", cfg.UploadToken)
	assert.Equal(t, "https://ldflag.example/api/v1/reports", cfg.IngestURL)
	assert.Equal(t, "ldflag@example.com", cfg.ITContactEmail)
	require.Len(t, cfg.POPs, 1)
	assert.Equal(t, "test-1", cfg.POPs[0].ID)
	assert.Equal(t, "test.example.com", cfg.POPs[0].Hostname)
}

func TestLoadWithEmptyLDFlagsLeavesSecretsEmpty(t *testing.T) {
	// With no ldflags injection (and the embedded defaults stripped of secrets),
	// Load("") returns empty secret fields and an empty POP list. Sidecar or
	// future ldflags inject can populate them.
	origToken := defaults.UploadToken
	origURL := defaults.IngestURL
	origEmail := defaults.ITContactEmail
	origPOPs := defaults.FallbackPOPsJSON
	t.Cleanup(func() {
		defaults.UploadToken = origToken
		defaults.IngestURL = origURL
		defaults.ITContactEmail = origEmail
		defaults.FallbackPOPsJSON = origPOPs
	})
	defaults.UploadToken = ""
	defaults.IngestURL = ""
	defaults.ITContactEmail = ""
	defaults.FallbackPOPsJSON = ""

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Empty(t, cfg.UploadToken)
	assert.Empty(t, cfg.IngestURL)
	assert.Empty(t, cfg.ITContactEmail)
	assert.Empty(t, cfg.POPs)
	// Non-secret tunables still come from the embed.
	assert.Equal(t, 15, cfg.PerTestTimeoutSeconds)
	assert.Equal(t, "PFLT", cfg.UDPProbeMagic)
}
