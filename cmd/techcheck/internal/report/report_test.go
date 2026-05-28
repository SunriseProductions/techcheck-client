package report_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestNewGeneratesRunID(t *testing.T) {
	r := report.New()
	assert.NotEmpty(t, r.RunID)
	assert.Equal(t, "techcheck.v1", r.Schema)
	assert.NotEmpty(t, r.ToolVersion)
}

func TestJSONRoundTrip(t *testing.T) {
	original := report.New()
	original.User.FullName = "Jane Doe"
	original.User.Email = "jane@example.com"
	original.Consent.Accepted = true
	original.Consent.At = time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	original.Machine.OS.Name = "Darwin"
	original.Network.PublicIP = "1.2.3.4"
	original.Network.POPs = append(original.Network.POPs, report.POPResult{
		ID:          "us-east-1",
		RegionLabel: "Virginia",
		Latency:     report.LatencyResult{Method: "icmp", MedianMS: 42},
		TCP443:      report.Reachability{Status: "reachable"},
	})

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed report.Report
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, original.RunID, parsed.RunID)
	assert.Equal(t, "Jane Doe", parsed.User.FullName)
	assert.Len(t, parsed.Network.POPs, 1)
	assert.True(t, parsed.RunStartedAt.Equal(original.RunStartedAt), "RunStartedAt must round-trip")
	assert.Equal(t, time.UTC, parsed.RunStartedAt.Location())
	assert.Equal(t, "icmp", parsed.Network.POPs[0].Latency.Method)
}

func TestNewInitialisesAllCollections(t *testing.T) {
	r := report.New()
	data, err := json.Marshal(r)
	require.NoError(t, err)

	s := string(data)
	// No null collections anywhere in a freshly-constructed report.
	assert.NotContains(t, s, `"errors":null`)
	assert.NotContains(t, s, `"pops":null`)
	assert.NotContains(t, s, `"gpus":null`)
	assert.NotContains(t, s, `"displays":null`)
	assert.NotContains(t, s, `"network_adapters":null`)
	assert.NotContains(t, s, `"vpn_detected":null`)
	assert.NotContains(t, s, `"system_proxy":null`)
}

func TestRecordErrorAppendsToList(t *testing.T) {
	r := report.New()
	r.RecordError("us-east-1", "udp_4172", "i/o timeout")
	require.Len(t, r.Errors, 1)
	assert.Equal(t, "us-east-1", r.Errors[0].POPID)
	assert.Equal(t, "udp_4172", r.Errors[0].Test)
	assert.Equal(t, "i/o timeout", r.Errors[0].Message)
}

func TestNewPopulatesAppID(t *testing.T) {
	r := report.New()
	if r.AppID != report.AppID {
		t.Fatalf("AppID = %q, want %q", r.AppID, report.AppID)
	}
	if r.AppID != "sunrise-techcheck" {
		t.Fatalf("AppID constant = %q, want %q", report.AppID, "sunrise-techcheck")
	}
}

func TestReportMarshalsAppID(t *testing.T) {
	r := report.New()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["app_id"] != "sunrise-techcheck" {
		t.Fatalf("json app_id = %v, want sunrise-techcheck", m["app_id"])
	}
}
