package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/defaults"
)

//go:embed defaults/defaults.json
var defaultsJSON []byte

type POP struct {
	ID          string `json:"id"`
	RegionLabel string `json:"region_label"`
	Hostname    string `json:"hostname"`
	UDPEcho     bool   `json:"udp_echo"`
}

type Config struct {
	IngestURL             string `json:"ingest_url"`
	UploadToken           string `json:"upload_token"`
	ITContactEmail        string `json:"it_contact_email"`
	PerTestTimeoutSeconds int    `json:"per_test_timeout_seconds"`
	UploadTimeoutSeconds  int    `json:"upload_timeout_seconds"`
	UploadRetries         int    `json:"upload_retries"`
	OmitWifiSSID          bool   `json:"omit_wifi_ssid"`
	UDPProbeMagic         string `json:"udp_probe_magic"`
	POPs                  []POP  `json:"pops"`
}

// Load merges baked-in defaults with build-time ldflags overrides and an
// optional sidecar JSON file at sidecarPath. Empty sidecarPath means
// defaults + ldflags overrides only.
//
// Precedence (lowest → highest):
//  1. Embedded defaults.json (non-secret tunables)
//  2. defaults.FallbackPOPsJSON (build-time injected fallback POP list)
//  3. defaults.{UploadToken, IngestURL, ITContactEmail} (build-time injected)
//  4. Sidecar JSON file at sidecarPath, if present
func Load(sidecarPath string) (*Config, error) {
	cfg := &Config{}
	if err := json.Unmarshal(defaultsJSON, cfg); err != nil {
		return nil, fmt.Errorf("parse baked-in defaults: %w", err)
	}

	// Apply build-time injected fallback POPs, if any.
	if defaults.FallbackPOPsJSON != "" {
		var pops []POP
		if err := json.Unmarshal([]byte(defaults.FallbackPOPsJSON), &pops); err != nil {
			return nil, fmt.Errorf("parse FallbackPOPsJSON: %w", err)
		}
		cfg.POPs = pops
	}

	// Apply build-time injected scalar overrides. Only non-empty values
	// override — that way an empty injection (public source build) doesn't
	// blank out a sidecar-provided value.
	if defaults.UploadToken != "" {
		cfg.UploadToken = defaults.UploadToken
	}
	if defaults.IngestURL != "" {
		cfg.IngestURL = defaults.IngestURL
	}
	if defaults.ITContactEmail != "" {
		cfg.ITContactEmail = defaults.ITContactEmail
	}

	if sidecarPath == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read sidecar %s: %w", sidecarPath, err)
	}
	defaultPOPs := cfg.POPs
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse sidecar %s: %w", sidecarPath, err)
	}
	if len(cfg.POPs) == 0 {
		cfg.POPs = defaultPOPs
	}
	return cfg, nil
}
