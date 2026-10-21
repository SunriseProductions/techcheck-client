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
	IngestURL             string      `json:"ingest_url"`
	UploadToken           string      `json:"upload_token"`
	ITContactEmail        string      `json:"it_contact_email"`
	PerTestTimeoutSeconds int         `json:"per_test_timeout_seconds"`
	UploadTimeoutSeconds  int         `json:"upload_timeout_seconds"`
	UploadRetries         int         `json:"upload_retries"`
	OmitWifiSSID          bool        `json:"omit_wifi_ssid"`
	UDPProbeMagic         string      `json:"udp_probe_magic"`
	POPs                  []POP       `json:"pops"`
	Run                   RunSettings `json:"run"`
}

// UDPStreamSettings tunes the DCV UDP stream test.
type UDPStreamSettings struct {
	PacketBytes     int       `json:"packet_bytes"`
	LadderMbps      []float64 `json:"ladder_mbps"`
	StepSeconds     float64   `json:"step_seconds"`
	LossLimitPct    float64   `json:"loss_limit_pct"`
	TeradiciMbps    float64   `json:"teradici_mbps"`
	TeradiciSeconds float64   `json:"teradici_seconds"`
}

// RunSettings holds the per-run limits and test parameters served by the
// POP endpoint.
type RunSettings struct {
	FullTestMaxLatencyMS float64           `json:"full_test_max_latency_ms"`
	FullTestMaxPOPs      int               `json:"full_test_max_pops"`
	TLSExpectedRoots     []string          `json:"tls_expected_roots"`
	DCVTCPPort           int               `json:"dcv_tcp_port"`
	DCVUDPPort           int               `json:"dcv_udp_port"`
	UDPStream            UDPStreamSettings `json:"udp_stream"`
}

// DefaultRunSettings returns the built-in run settings. Slices are fresh on
// every call, so callers may mutate the result.
func DefaultRunSettings() RunSettings {
	return RunSettings{
		FullTestMaxLatencyMS: 100,
		FullTestMaxPOPs:      3,
		TLSExpectedRoots:     []string{"ISRG Root X1", "ISRG Root X2"},
		DCVTCPPort:           8443,
		DCVUDPPort:           8443,
		UDPStream: UDPStreamSettings{
			PacketBytes:     1250,
			LadderMbps:      []float64{5, 10, 20, 35},
			StepSeconds:     3,
			LossLimitPct:    0.1,
			TeradiciMbps:    5,
			TeradiciSeconds: 5,
		},
	}
}

// WithDefaults returns r with every zero-valued or empty field replaced by
// its built-in default.
func (r RunSettings) WithDefaults() RunSettings {
	d := DefaultRunSettings()
	if r.FullTestMaxLatencyMS == 0 {
		r.FullTestMaxLatencyMS = d.FullTestMaxLatencyMS
	}
	if r.FullTestMaxPOPs == 0 {
		r.FullTestMaxPOPs = d.FullTestMaxPOPs
	}
	if len(r.TLSExpectedRoots) == 0 {
		r.TLSExpectedRoots = d.TLSExpectedRoots
	}
	if r.DCVTCPPort == 0 {
		r.DCVTCPPort = d.DCVTCPPort
	}
	if r.DCVUDPPort == 0 {
		r.DCVUDPPort = d.DCVUDPPort
	}
	u := &r.UDPStream
	if u.PacketBytes == 0 {
		u.PacketBytes = d.UDPStream.PacketBytes
	}
	if len(u.LadderMbps) == 0 {
		u.LadderMbps = d.UDPStream.LadderMbps
	}
	if u.StepSeconds == 0 {
		u.StepSeconds = d.UDPStream.StepSeconds
	}
	if u.LossLimitPct == 0 {
		u.LossLimitPct = d.UDPStream.LossLimitPct
	}
	if u.TeradiciMbps == 0 {
		u.TeradiciMbps = d.UDPStream.TeradiciMbps
	}
	if u.TeradiciSeconds == 0 {
		u.TeradiciSeconds = d.UDPStream.TeradiciSeconds
	}
	return r
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
	cfg := &Config{Run: DefaultRunSettings()}
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
