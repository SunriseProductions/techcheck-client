package sysinfo

// Pure parsers for the JSON the Windows PowerShell scripts emit. Kept
// build-tag-free so they compile (and are unit-tested) on dev macs even
// though `peripherals_windows.go` is the only caller.

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// psGPU mirrors one Win32_VideoController row. WidthPX/HeightPX/RefreshHz
// are the *current display mode of the adapter's attached display* — kept on
// this struct only so parseDisplaysAndGPUs can join refresh back to each
// Screen.AllScreens entry by resolution.
type psGPU struct {
	Name       string `json:"Name"`
	AdapterRAM int64  `json:"AdapterRAM"`
	WidthPX    int    `json:"WidthPX"`
	HeightPX   int    `json:"HeightPX"`
	RefreshHz  int    `json:"RefreshHz"`
}

type psScreen struct {
	WidthPX  int  `json:"WidthPX"`
	HeightPX int  `json:"HeightPX"`
	Primary  bool `json:"Primary"`
}

type psDisplaysGPUs struct {
	GPUs    []psGPU    `json:"GPUs"`
	Screens []psScreen `json:"Screens"`
}

type psPnpEntity struct {
	Name         string `json:"Name"`
	Manufacturer string `json:"Manufacturer"`
	PNPDeviceID  string `json:"PNPDeviceID"`
}

// VID/PID live in PNPDeviceID strings shaped like
// `USB\VID_05AC&PID_8600\<serial>` (case can vary).
var usbVidPidRe = regexp.MustCompile(`(?i)^USB\\VID_([0-9A-F]{4})&PID_([0-9A-F]{4})`)

// parseDisplaysAndGPUs joins the two PowerShell views and emits one
// report.Display per Screen.AllScreens entry. Refresh rate is best-effort:
// when two screens share a resolution and only one GPU mode is reported,
// both screens get that refresh — the controller only surfaces one current
// mode per adapter, so the second value is a copy, not a measurement.
func parseDisplaysAndGPUs(out []byte) ([]report.GPU, []report.Display) {
	var parsed psDisplaysGPUs
	if err := json.Unmarshal(out, &parsed); err != nil {
		return []report.GPU{}, []report.Display{}
	}

	gpus := make([]report.GPU, 0, len(parsed.GPUs))
	for _, g := range parsed.GPUs {
		if g.Name == "" {
			continue
		}
		// AdapterRAM is a uint32 in WMI and caps at 4 GiB for GPUs with more
		// VRAM. Better than nothing for the IT-readiness check.
		var vram uint64
		if g.AdapterRAM > 0 {
			vram = uint64(g.AdapterRAM)
		}
		gpus = append(gpus, report.GPU{Model: g.Name, VRAMBytes: vram})
	}

	displays := make([]report.Display, 0, len(parsed.Screens))
	for _, s := range parsed.Screens {
		if s.WidthPX <= 0 || s.HeightPX <= 0 {
			continue
		}
		d := report.Display{WidthPX: s.WidthPX, HeightPX: s.HeightPX}
		for _, g := range parsed.GPUs {
			if g.WidthPX == s.WidthPX && g.HeightPX == s.HeightPX && g.RefreshHz > 0 {
				d.RefreshHz = g.RefreshHz
				break
			}
		}
		displays = append(displays, d)
	}
	return gpus, displays
}

// parseUSBDevices accepts either a JSON array (multi-result) or a single
// JSON object (PowerShell's single-result quirk, in case the @(...) wrapper
// is ever lost).
func parseUSBDevices(out []byte) []report.USBDevice {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return []report.USBDevice{}
	}
	var entries []psPnpEntity
	if trimmed[0] == '[' {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return []report.USBDevice{}
		}
	} else {
		var single psPnpEntity
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return []report.USBDevice{}
		}
		entries = []psPnpEntity{single}
	}

	devices := make([]report.USBDevice, 0, len(entries))
	for _, e := range entries {
		m := usbVidPidRe.FindStringSubmatch(e.PNPDeviceID)
		if m == nil {
			continue
		}
		devices = append(devices, report.USBDevice{
			Name:         e.Name,
			Manufacturer: e.Manufacturer,
			VendorID:     "0x" + strings.ToLower(m[1]),
			ProductID:    "0x" + strings.ToLower(m[2]),
		})
	}
	return devices
}
