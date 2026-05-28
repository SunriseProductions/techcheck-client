//go:build darwin

package sysinfo

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// wirelessInterfaceNames returns the set of interface names (e.g. "en0")
// that macOS reports as Wi-Fi hardware ports. Used to override the
// name-based adapter classifier, since macOS's Wi-Fi interface isn't named
// "wl*" or "wifi*".
func wirelessInterfaceNames(ctx context.Context) map[string]bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "networksetup", "-listallhardwareports").Output()
	if err != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	var currentIsWifi bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "Hardware Port:"):
			port := strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
			currentIsWifi = strings.EqualFold(port, "Wi-Fi") || strings.EqualFold(port, "AirPort")
		case strings.HasPrefix(line, "Device:"):
			if currentIsWifi {
				dev := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
				if dev != "" {
					result[dev] = true
				}
			}
		}
	}
	return result
}

// collectDisplaysAndGPUs runs `system_profiler SPDisplaysDataType -json` and
// extracts the GPU list and connected-display list. Best-effort: any parse
// failure returns empty slices.
func collectDisplaysAndGPUs(ctx context.Context) ([]report.GPU, []report.Display) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "system_profiler", "SPDisplaysDataType", "-json").Output()
	if err != nil {
		return []report.GPU{}, []report.Display{}
	}

	var parsed struct {
		Data []map[string]any `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return []report.GPU{}, []report.Display{}
	}

	gpus := make([]report.GPU, 0, len(parsed.Data))
	displays := []report.Display{}
	for _, entry := range parsed.Data {
		gpu := report.GPU{}
		if name, _ := entry["_name"].(string); name != "" {
			gpu.Model = name
		}
		// VRAM lives in one of several keys depending on Mac model.
		// On Apple Silicon (unified memory) it's often absent.
		for _, key := range []string{"spdisplays_vram", "spdisplays_vram_shared", "sppci_vram"} {
			if v, _ := entry[key].(string); v != "" {
				gpu.VRAMBytes = parseBytesSuffix(v)
				break
			}
		}
		if gpu.Model != "" {
			gpus = append(gpus, gpu)
		}

		ndrvs, _ := entry["spdisplays_ndrvs"].([]any)
		for _, n := range ndrvs {
			nm, _ := n.(map[string]any)
			if nm == nil {
				continue
			}
			d := report.Display{}
			if pixels, _ := nm["_spdisplays_pixels"].(string); pixels != "" {
				w, h := parsePixels(pixels)
				d.WidthPX = w
				d.HeightPX = h
			}
			if res, _ := nm["_spdisplays_resolution"].(string); res != "" {
				d.RefreshHz = parseRefresh(res)
			}
			if d.WidthPX > 0 || d.HeightPX > 0 {
				displays = append(displays, d)
			}
		}
	}
	return gpus, displays
}

// collectUSBDevices runs `system_profiler SPUSBHostDataType -json` and
// extracts one report.USBDevice per attached peripheral. Built-in host
// controllers appear as root nodes without a vendor ID and are skipped;
// everything in the _items tree that looks like a real device is captured.
// Serial numbers are deliberately not surfaced — they're a stable hardware
// identifier and aren't needed for an IT tech check.
func collectUSBDevices(ctx context.Context) []report.USBDevice {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// SPUSBHostDataType is the modern name (Ventura+). SPUSBDataType is the
	// legacy name kept for older macOS; try both and take whichever returns
	// a non-empty tree.
	for _, dataType := range []string{"SPUSBHostDataType", "SPUSBDataType"} {
		out, err := exec.CommandContext(ctx, "system_profiler", dataType, "-json").Output()
		if err != nil {
			continue
		}
		var parsed map[string][]map[string]any
		if err := json.Unmarshal(out, &parsed); err != nil {
			continue
		}
		devices := []report.USBDevice{}
		for _, bus := range parsed[dataType] {
			walkUSBItems(bus["_items"], &devices)
		}
		if len(devices) > 0 {
			return devices
		}
	}
	return []report.USBDevice{}
}

// walkUSBItems walks a system_profiler _items tree and appends one USBDevice
// per node that carries a vendor ID (i.e. a real peripheral, not a bus). Hubs
// and their children are both captured — the hub is informative in its own
// right for diagnosing bus-power issues.
func walkUSBItems(items any, out *[]report.USBDevice) {
	list, _ := items.([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		if d, ok := usbDeviceFromNode(m); ok {
			*out = append(*out, d)
		}
		if kids, ok := m["_items"]; ok {
			walkUSBItems(kids, out)
		}
	}
}

// usbDeviceFromNode extracts a USBDevice from a system_profiler node. Returns
// ok=false for bus/root nodes (no vendor ID). Handles both the modern
// USBDeviceKey* field names and the legacy lowercase names.
func usbDeviceFromNode(m map[string]any) (report.USBDevice, bool) {
	vendorID := firstString(m, "USBDeviceKeyVendorID", "vendor_id")
	if vendorID == "" {
		return report.USBDevice{}, false
	}
	return report.USBDevice{
		Name:         firstString(m, "_name"),
		Manufacturer: firstString(m, "USBDeviceKeyVendorName", "manufacturer"),
		VendorID:     vendorID,
		ProductID:    firstString(m, "USBDeviceKeyProductID", "product_id"),
		Speed:        firstString(m, "USBDeviceKeyLinkSpeed", "speed"),
	}, true
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, _ := m[k].(string); v != "" {
			return v
		}
	}
	return ""
}

// parseBytesSuffix converts strings like "8 GB", "1536 MB", "32 GB" into bytes.
// Returns 0 on failure.
func parseBytesSuffix(s string) uint64 {
	re := regexp.MustCompile(`(?i)^\s*([0-9.]+)\s*([KMGT]?B)?\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mult := uint64(1)
	switch strings.ToUpper(m[2]) {
	case "KB":
		mult = 1024
	case "MB":
		mult = 1024 * 1024
	case "GB":
		mult = 1024 * 1024 * 1024
	case "TB":
		mult = 1024 * 1024 * 1024 * 1024
	}
	return uint64(n * float64(mult))
}

// parsePixels extracts W, H from strings like "3456 x 2234" or
// "3456 × 2234". Returns 0, 0 on failure.
func parsePixels(s string) (int, int) {
	re := regexp.MustCompile(`([0-9]+)\s*[x×]\s*([0-9]+)`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, 0
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	return w, h
}

// parseRefresh extracts Hz from strings like "3456 x 2234 @ 120.00Hz".
func parseRefresh(s string) int {
	re := regexp.MustCompile(`@\s*([0-9.]+)\s*Hz`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	return int(n)
}
