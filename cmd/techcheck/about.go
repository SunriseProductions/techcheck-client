package main

import (
	_ "embed"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed build/appicon.png
var appIcon []byte

// aboutMessage builds the body of the macOS About panel. The version is taken
// from report.ToolVersion (the single source of truth) so it can never drift
// from what the app reports to ingest. The support line is omitted when no
// contact email is configured.
func aboutMessage(version, supportEmail string) string {
	msg := fmt.Sprintf(
		"Version %s\nNetwork readiness diagnostic\n\n"+
			"Created by Tim Keller\nSunrise Animation Studios\n"+
			"Copyright © 2026 Sunrise Animation Studios",
		version,
	)
	if supportEmail != "" {
		msg += "\n\nSupport: " + supportEmail
	}
	return msg
}

// aboutInfo returns the Wails macOS About config. Setting it makes Wails add the
// native "About Sunrise Tech Check" item to the app menu and show the panel.
func aboutInfo(version, supportEmail string) *mac.AboutInfo {
	return &mac.AboutInfo{
		Title:   "Sunrise Tech Check",
		Message: aboutMessage(version, supportEmail),
		Icon:    appIcon,
	}
}
