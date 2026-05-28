package sysinfo_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/sysinfo"
)

func TestCollectBasics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	m, err := sysinfo.Collect(ctx, sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)

	assert.NotEmpty(t, m.OS.Name)
	assert.NotEmpty(t, m.OS.Arch)
	assert.Equal(t, runtime.GOARCH, archReportToGo(m.OS.Arch))
	assert.NotEmpty(t, m.Hostname)
	assert.Greater(t, m.CPU.LogicalCores, 0)
	assert.Greater(t, m.Memory.TotalBytes, uint64(0))
	assert.Greater(t, m.Storage.SystemVolumeFreeBytes, uint64(0))
	assert.NotEmpty(t, m.Timezone)
}

func TestCollectCPUCoreCountsAreSane(t *testing.T) {
	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{})
	require.NoError(t, err)
	assert.Greater(t, m.CPU.PhysicalCores, 0)
	assert.Greater(t, m.CPU.LogicalCores, 0)
	assert.LessOrEqual(t, m.CPU.PhysicalCores, m.CPU.LogicalCores,
		"physical cores cannot exceed logical cores (SMT/HT)")
}

func TestCollectPower(t *testing.T) {
	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)

	// On desktops without a battery, OnAC is true and BatteryPercent is 100.
	// On laptops the value is read from the platform battery API. Either way,
	// BatteryPercent must fall in [0, 100].
	assert.GreaterOrEqual(t, m.Power.BatteryPercent, 0)
	assert.LessOrEqual(t, m.Power.BatteryPercent, 100)
}

func TestCollectNetworkAdapters(t *testing.T) {
	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)

	assert.NotNil(t, m.NetworkAdapters)
	for _, a := range m.NetworkAdapters {
		assert.Contains(t, []string{"wired", "wireless"}, a.Type,
			"adapter type must be wired or wireless")
	}
}

// TestOmitWifiSSID asserts the omit contract. In v1 the SSID is always "",
// so this test exercises the invariant at type level. When v1.1 adds a real
// platform SSID lookup, this test becomes the regression guard for the omit
// flag.
func TestOmitWifiSSID(t *testing.T) {
	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)
	for _, a := range m.NetworkAdapters {
		assert.Empty(t, a.WifiSSID, "SSID must be omitted when OmitWifiSSID is true")
	}
}

func TestCollectSecurityPosture(t *testing.T) {
	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)

	// Fields are populated by cross-platform + platform-specific code. We
	// assert the non-null-slice shape per the JSON contract.
	assert.NotNil(t, m.Security.VPNDetected)
	assert.NotNil(t, m.Security.SystemProxy)
}

func TestDetectSystemProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://proxy.example:3128")
	t.Setenv("HTTPS_PROXY", "https://proxy.example:3129")

	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)
	assert.Equal(t, "http://proxy.example:3128", m.Security.SystemProxy["http"])
	assert.Equal(t, "https://proxy.example:3129", m.Security.SystemProxy["https"])
}

func TestDetectSystemProxyLowercaseEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "http://lower.example:3128")

	m, err := sysinfo.Collect(context.Background(), sysinfo.Options{OmitWifiSSID: true})
	require.NoError(t, err)
	assert.Equal(t, "http://lower.example:3128", m.Security.SystemProxy["http"])
}

// archReportToGo inverts the "report-style" arch string to runtime.GOARCH.
func archReportToGo(arch string) string {
	switch arch {
	case "x64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "x86":
		return "386"
	}
	return arch
}
