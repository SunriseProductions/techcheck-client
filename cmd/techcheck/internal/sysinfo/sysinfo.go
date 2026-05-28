package sysinfo

import (
	"context"
	"fmt"
	"math"
	gonet "net"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/distatus/battery"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// Options controls sysinfo collection. OmitWifiSSID is consumed by the
// network adapter collector (added in Task 6); it's declared here so the
// Collect signature is stable across tasks.
type Options struct {
	OmitWifiSSID bool
}

// Collect gathers machine information into a report.Machine. Individual
// field failures are tolerated (left as zero values); Collect only returns
// an error if the OS refuses to give us any host info at all.
func Collect(ctx context.Context, opts Options) (report.Machine, error) {
	// Pre-initialise collections so Machine marshals to `[]` / `{}` rather than
	// null. Callers that obtain Machine via report.New() already have these
	// initialised, but Collect is also used standalone and must carry its own
	// contract.
	m := report.Machine{
		GPUs:            []report.GPU{},
		Displays:        []report.Display{},
		NetworkAdapters: []report.NetworkAdapter{},
		Peripherals: report.Peripherals{
			USBDevices: []report.USBDevice{},
		},
		Security: report.Security{
			VPNDetected: []string{},
			SystemProxy: map[string]string{},
		},
	}

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return m, fmt.Errorf("host.Info: %w", err)
	}
	m.OS.Name = info.Platform
	m.OS.Version = info.PlatformVersion
	m.OS.Build = info.KernelVersion
	m.OS.Arch = normaliseArch(runtime.GOARCH)
	m.MachineID = info.HostID
	m.Hostname = info.Hostname
	if u, err := user.Current(); err == nil {
		m.LoggedInUsername = u.Username
	}
	if loc := time.Now().Location(); loc != nil {
		m.Timezone = loc.String()
	}
	// Locale is best-effort from $LANG; on macOS/Windows the user-facing locale
	// lives in platform APIs and may differ. Good enough for v1 IT diagnostic use.
	if loc := os.Getenv("LANG"); loc != "" {
		m.Locale = loc
	}

	cpuInfos, err := cpu.InfoWithContext(ctx)
	if err == nil && len(cpuInfos) > 0 {
		m.CPU.Model = cpuInfos[0].ModelName
		m.CPU.BaseFrequencyMHz = int(cpuInfos[0].Mhz)
	}
	if physical, err := cpu.CountsWithContext(ctx, false); err == nil {
		m.CPU.PhysicalCores = physical
	}
	if logical, err := cpu.CountsWithContext(ctx, true); err == nil {
		m.CPU.LogicalCores = logical
	}

	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		m.Memory.TotalBytes = v.Total
	}

	systemPath := "/"
	if runtime.GOOS == "windows" {
		systemPath = "C:"
	}
	if u, err := disk.UsageWithContext(ctx, systemPath); err == nil {
		m.Storage.SystemVolumeFreeBytes = u.Free
	}

	// Partial virtualisation — gopsutil's host.Info surfaces this.
	if info.VirtualizationSystem != "" {
		m.Virtualisation.IsVM = info.VirtualizationRole == "guest"
		m.Virtualisation.Hypervisor = info.VirtualizationSystem
	}

	m.Power = collectPower()

	m.NetworkAdapters = collectAdapters(ctx, opts.OmitWifiSSID)

	// Displays, GPUs, USB: macOS via system_profiler, Windows via PowerShell
	// (Win32_VideoController + Screen.AllScreens + Win32_PnPEntity). Linux is
	// still stubbed; tracked as v1.1 work.
	m.GPUs, m.Displays = collectDisplaysAndGPUs(ctx)
	m.Peripherals.USBDevices = collectUSBDevices(ctx)
	m.Peripherals.USBDeviceCount = len(m.Peripherals.USBDevices)

	// VPN detection via process enumeration was removed to keep the macOS
	// binary past XProtect. VPNDetected stays as the empty slice initialised
	// above. Real detection would require signed/notarised builds or
	// platform-specific privileged APIs; revisit with code signing (PRD §8.2).
	m.Security.SystemProxy = detectSystemProxy()
	collectSecurity(&m.Security) // platform-specific firewall / AV

	return m, nil
}

func normaliseArch(goArch string) string {
	switch goArch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	}
	return goArch
}

// collectPower reads the first attached battery via distatus/battery. On
// desktops with no battery it returns OnAC=true and BatteryPercent=100 — the
// interpretation is "fully powered, never going to die from running out of
// battery during a preflight run."
func collectPower() report.Power {
	batteries, err := battery.GetAll()
	if len(batteries) == 0 {
		_ = err // no usable readings — treat as desktop.
		return report.Power{OnAC: true, BatteryPercent: 100}
	}
	b := batteries[0]
	pct := 100
	if b.Full > 0 {
		pct = int(math.Round((b.Current / b.Full) * 100))
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	onAC := false
	switch b.State.Raw {
	case battery.Charging, battery.Full, battery.Idle:
		onAC = true
	case battery.Unknown, battery.Undefined:
		// Sensor state unclear — assume on AC. Safer for a preflight diagnostic
		// than a false "battery risk" signal.
		onAC = true
	}
	return report.Power{OnAC: onAC, BatteryPercent: pct}
}

// collectAdapters enumerates the machine's up, non-loopback network
// interfaces. The wired/wireless classification is a heuristic based on
// interface name; it's not authoritative but accurate in practice for the
// common cases (e.g. "en0" on macOS, "wlan0" on Linux, "Wi-Fi" on Windows).
// A future v1.1 task may use platform APIs for an authoritative answer.
func collectAdapters(ctx context.Context, omitSSID bool) []report.NetworkAdapter {
	ifs, err := gonet.Interfaces()
	if err != nil {
		return []report.NetworkAdapter{}
	}

	// Build a name → gopsutil stats map for link-speed lookup.
	stats, _ := psnet.IOCountersWithContext(ctx, true)
	byName := make(map[string]psnet.IOCountersStat, len(stats))
	for _, s := range stats {
		byName[s.Name] = s
	}

	wirelessNames := wirelessInterfaceNames(ctx)

	adapters := []report.NetworkAdapter{}
	for _, i := range ifs {
		if i.Flags&gonet.FlagLoopback != 0 {
			continue
		}
		if i.Flags&gonet.FlagUp == 0 {
			continue
		}
		// Skip interfaces that don't carry real traffic. Without this macOS
		// reports ~28 adapters (awdl, llw, utun, bridge, anpi, thunderbolt
		// stubs...) that are "up" but have no usable address.
		if !hasUsableIP(i) {
			continue
		}
		kind := classifyAdapter(i.Name)
		if wirelessNames[i.Name] {
			kind = report.NetworkAdapterWireless
		}
		adapters = append(adapters, report.NetworkAdapter{
			Type:          kind,
			LinkSpeedMbps: 0,
			IsDefault:     false,
			WifiSSID:      readWifiSSIDIfWanted(i.Name, omitSSID),
		})
	}
	_ = byName // TODO(v1.1): wire byName[iface].Speed into LinkSpeedMbps.

	// Mark the first wired adapter (or first wireless if no wired) as default.
	// gopsutil doesn't surface the default route cheaply on all platforms, so
	// this heuristic is the v1 answer. Good enough for IT to spot the primary
	// link in a report.
	defaultIdx := -1
	for idx, a := range adapters {
		if a.Type == "wired" {
			defaultIdx = idx
			break
		}
	}
	if defaultIdx == -1 {
		for idx, a := range adapters {
			if a.Type == "wireless" {
				defaultIdx = idx
				break
			}
		}
	}
	if defaultIdx >= 0 {
		adapters[defaultIdx].IsDefault = true
	}

	return adapters
}

// hasUsableIP reports whether the interface has at least one globally
// routable IP (non-loopback, non-link-local, non-unspecified). This filters
// out the zoo of macOS virtual interfaces that are "up" but carry no
// external traffic (awdl0, llw0, utunN, bridgeN, anpiN, ...).
func hasUsableIP(i gonet.Interface) bool {
	addrs, err := i.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		var ip gonet.IP
		switch v := a.(type) {
		case *gonet.IPNet:
			ip = v.IP
		case *gonet.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		return true
	}
	return false
}

// classifyAdapter returns "wireless" for interface names that look like
// Wi-Fi adapters; everything else is classified "wired". Heuristic only.
func classifyAdapter(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "wl") { // Linux: wlan0, wlp3s0, wlx…
		return "wireless"
	}
	if strings.Contains(lower, "wifi") ||
		strings.Contains(lower, "wi-fi") ||
		strings.Contains(lower, "airport") {
		return "wireless"
	}
	return "wired"
}

// readWifiSSIDIfWanted returns the SSID for a wireless interface, or "" if
// the adapter isn't wireless, the user opted out, or the platform hasn't
// implemented SSID lookup yet. v1 returns "" unconditionally — actual SSID
// reading is v1.1 platform work.
func readWifiSSIDIfWanted(name string, omit bool) string {
	if omit {
		return ""
	}
	if classifyAdapter(name) != "wireless" {
		return ""
	}
	return "" // TODO(v1.1): platform-specific SSID lookup.
}

// detectSystemProxy reads the standard HTTP(S) proxy env vars. Platform-
// specific code may override with OS-level proxy settings via collectSecurity.
func detectSystemProxy() map[string]string {
	p := map[string]string{}
	if v := firstNonEmpty(os.Getenv("HTTP_PROXY"), os.Getenv("http_proxy")); v != "" {
		p["http"] = v
	}
	if v := firstNonEmpty(os.Getenv("HTTPS_PROXY"), os.Getenv("https_proxy")); v != "" {
		p["https"] = v
	}
	return p
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
