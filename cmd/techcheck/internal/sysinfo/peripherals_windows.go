//go:build windows

package sysinfo

import (
	"context"
	"os/exec"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// wirelessInterfaceNames returns an empty set on Windows; the name-based
// classifier ("Wi-Fi") in classifyAdapter is sufficient.
func wirelessInterfaceNames(ctx context.Context) map[string]bool {
	return map[string]bool{}
}

// collectDisplaysAndGPUs shells out to PowerShell and parses the combined
// Win32_VideoController + System.Windows.Forms.Screen.AllScreens output.
//
// Win32_VideoController gives GPU model, VRAM, and the current display mode
// of the adapter's attached display. System.Windows.Forms.Screen.AllScreens
// gives accurate per-monitor bounds — important for multi-monitor setups
// where a single adapter drives more than one display. Refresh rate is
// matched back to each screen by resolution; when no controller matches,
// RefreshHz stays 0. Best-effort: any failure returns empty slices.
func collectDisplaysAndGPUs(ctx context.Context) ([]report.GPU, []report.Display) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const script = `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Add-Type -AssemblyName System.Windows.Forms | Out-Null
$gpus = @(Get-CimInstance Win32_VideoController | ForEach-Object {
    [PSCustomObject]@{
        Name        = $_.Name
        AdapterRAM  = [int64]$_.AdapterRAM
        WidthPX     = [int]$_.CurrentHorizontalResolution
        HeightPX    = [int]$_.CurrentVerticalResolution
        RefreshHz   = [int]$_.CurrentRefreshRate
    }
})
$screens = @([System.Windows.Forms.Screen]::AllScreens | ForEach-Object {
    [PSCustomObject]@{
        WidthPX = [int]$_.Bounds.Width
        HeightPX = [int]$_.Bounds.Height
        Primary = [bool]$_.Primary
    }
})
[PSCustomObject]@{ GPUs = $gpus; Screens = $screens } | ConvertTo-Json -Depth 5 -Compress
`
	out, err := runPowerShell(ctx, script)
	if err != nil {
		return []report.GPU{}, []report.Display{}
	}
	return parseDisplaysAndGPUs(out)
}

// collectUSBDevices shells out to PowerShell for Win32_PnPEntity entries
// whose PNPDeviceID starts with `USB\`, then parses VID/PID out of the ID.
// Speed is omitted — Win32_PnPEntity doesn't carry it and joining via
// Win32_USBHub is too brittle for the value it adds.
func collectUSBDevices(ctx context.Context) []report.USBDevice {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const script = `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
@(Get-CimInstance Win32_PnPEntity | Where-Object { $_.PNPDeviceID -like 'USB\*' } | ForEach-Object {
    [PSCustomObject]@{
        Name        = $_.Name
        Manufacturer = $_.Manufacturer
        PNPDeviceID = $_.PNPDeviceID
    }
}) | ConvertTo-Json -Depth 3 -Compress
`
	out, err := runPowerShell(ctx, script)
	if err != nil {
		return []report.USBDevice{}
	}
	return parseUSBDevices(out)
}

// runPowerShell invokes powershell.exe with the given script. -NoProfile
// keeps startup fast and predictable; -NonInteractive guards against
// blocking on a prompt; hideWindow stops a black cmd window flashing in
// front of the Wails GUI on every sysinfo run.
func runPowerShell(ctx context.Context, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-Command", script)
	hideWindow(cmd)
	return cmd.Output()
}
