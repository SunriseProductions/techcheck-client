//go:build windows

package sysinfo

import (
	"os/exec"
	"strings"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// collectSecurity on Windows uses PowerShell shell-outs to read Windows
// Security Center. Best-effort; each field stays at its zero value if the
// PowerShell call fails. We run with -NoProfile to avoid user rc-script
// delays. All commands are static — no user input is interpolated.
func collectSecurity(s *report.Security) {
	// AV — ask the Security Center for the registered AV product.
	prod := exec.Command("powershell", "-NoProfile", "-Command",
		`(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct | Select-Object -First 1).displayName`)
	hideWindow(prod)
	if pb, err := prod.Output(); err == nil {
		if name := strings.TrimSpace(string(pb)); name != "" {
			s.AVProduct = name
		}
	}

	// Firewall — any profile enabled?
	fw := exec.Command("powershell", "-NoProfile", "-Command",
		"(Get-NetFirewallProfile | Where-Object { $_.Enabled -eq $true } | Measure-Object).Count -gt 0")
	hideWindow(fw)
	if out, err := fw.Output(); err == nil && strings.TrimSpace(string(out)) == "True" {
		s.FirewallEnabled = true
	}
}
