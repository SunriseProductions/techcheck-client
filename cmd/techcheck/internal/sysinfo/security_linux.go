//go:build linux

package sysinfo

import (
	"os/exec"
	"strings"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// collectSecurity on Linux checks ufw first (common on desktops), then falls
// back to iptables policy inspection. AV is not attempted on Linux — not
// meaningful in the Preflight target audience.
func collectSecurity(s *report.Security) {
	if out, err := exec.Command("ufw", "status").Output(); err == nil {
		if strings.Contains(string(out), "Status: active") {
			s.FirewallEnabled = true
			return
		}
	}
	ipt := exec.Command("iptables", "-L")
	if out, err := ipt.Output(); err == nil && len(out) > 0 {
		s.FirewallEnabled = strings.Contains(string(out), "policy DROP") ||
			strings.Contains(string(out), "policy REJECT")
	}
}
