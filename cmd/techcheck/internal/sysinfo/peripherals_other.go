//go:build !darwin && !windows

package sysinfo

import (
	"context"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// Stub implementations on platforms without a native peripherals layer
// (currently: Linux). macOS lives in peripherals_darwin.go, Windows in
// peripherals_windows.go. Linux is still tracked as v1.1 work.
func collectDisplaysAndGPUs(ctx context.Context) ([]report.GPU, []report.Display) {
	return []report.GPU{}, []report.Display{}
}

func collectUSBDevices(ctx context.Context) []report.USBDevice {
	return []report.USBDevice{}
}

// wirelessInterfaceNames returns an empty set on non-darwin; the name-based
// classifier is sufficient on Linux ("wl*") and Windows ("Wi-Fi").
func wirelessInterfaceNames(ctx context.Context) map[string]bool {
	return map[string]bool{}
}
