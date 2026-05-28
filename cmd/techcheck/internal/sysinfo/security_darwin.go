//go:build darwin

package sysinfo

import "github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"

// collectSecurity on macOS is intentionally a no-op in v1. Reading firewall
// state via socketfilterfw requires shelling out to
// /usr/libexec/ApplicationFirewall, which combined with the unsigned binary
// tripped macOS XProtect heuristics during local testing. Revisit once the
// binary is code-signed and notarised (PRD §8.2).
func collectSecurity(*report.Security) {}
