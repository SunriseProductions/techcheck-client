# Version is the single source of truth, read from the Go source so
# bundle/DMG/EXE filenames stay aligned with what the app reports to ingest.
VERSION    := $(shell grep -E '^[[:space:]]+ToolVersion[[:space:]]+=' cmd/techcheck/internal/report/report.go | sed -E 's/.*"([^"]+)".*/\1/')
NAME       := SunriseTechCheck
WAILS_DIR  := cmd/techcheck
BIN        := $(WAILS_DIR)/build/bin
DIST       := build/dist
INSTALL    := $(WAILS_DIR)/build/install
WAILS      := $(HOME)/go/bin/wails
PKG        := github.com/sunriseproductions/techcheck-client

# DMG layout: 540x380 window, app icon left, README top, Applications drop right.
DMG_WIN_W  := 540
DMG_WIN_H  := 380
DMG_ICON_PX := 96

# Source build-time secrets from private/secrets.env (relative to this
# Makefile) if it exists. Env vars from a parent process (e.g., a wrapping
# Makefile in the integration repo) take precedence and skip the include
# automatically — Make picks them up from the environment.
ifneq (,$(wildcard private/secrets.env))
  include private/secrets.env
  export UPLOAD_TOKEN INGEST_URL IT_CONTACT_EMAIL FALLBACK_POPS_JSON \
         APPLE_SIGN_IDENTITY APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD
endif

ifeq (,$(strip $(UPLOAD_TOKEN)$(INGEST_URL)$(IT_CONTACT_EMAIL)$(FALLBACK_POPS_JSON)))
  $(warning No build-time secrets supplied — building unauthenticated artifacts)
endif

# macOS code signing. Default is ad-hoc ("-") so local builds work without an
# Apple Developer cert installed. Set APPLE_SIGN_IDENTITY to the full identity
# string (e.g. "Developer ID Application: Sunrise Productions Ltd (TEAMID)")
# to produce a distributable, notarisable signature.
SIGN_IDENTITY := $(if $(strip $(APPLE_SIGN_IDENTITY)),$(APPLE_SIGN_IDENTITY),-)
ENTITLEMENTS  := $(WAILS_DIR)/build/darwin/entitlements.plist
# --timestamp requires the real cert + Apple's timestamp server; ad-hoc can't
# sign with timestamp. Entitlements still apply to ad-hoc — useful for
# verifying the entitlement file is correct locally before the cert arrives.
ifneq ($(SIGN_IDENTITY),-)
  CODESIGN_FLAGS := --force --options runtime --timestamp --entitlements $(ENTITLEMENTS)
else
  CODESIGN_FLAGS := --force --options runtime --entitlements $(ENTITLEMENTS)
endif

# LDFLAGS uses shell-side variable expansion ($$VAR → $VAR at make parse
# time; shell expands at recipe time). Make-side $(VAR) would inline the
# raw value, and FALLBACK_POPS_JSON contains double quotes that would
# break the surrounding shell quoting. Deferring to shell-time keeps the
# JSON's quotes as literal data inside the expanded value.
LDFLAGS := -X '$(PKG)/cmd/techcheck/internal/config/defaults.UploadToken=$$UPLOAD_TOKEN' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.IngestURL=$$INGEST_URL' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.ITContactEmail=$$IT_CONTACT_EMAIL' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.FallbackPOPsJSON=$$FALLBACK_POPS_JSON'

.PHONY: version test \
        build-mac-arm64 build-mac-x64 build-windows-x64 \
        sign-mac-arm64  sign-mac-x64 \
        dist-mac-arm64  dist-mac-x64  dist-windows-x64 \
        notarize-mac-arm64 notarize-mac-x64 \
        build-desktop release-desktop clean-dist verify-sign-mac

version:
	@echo "VERSION=$(VERSION)"

test:
	go test ./...

clean-dist:
	rm -rf $(DIST)
	mkdir -p $(DIST)

build-mac-arm64:
	cd $(WAILS_DIR) && $(WAILS) build -platform darwin/arm64 -clean -ldflags "$(LDFLAGS)"

build-mac-x64:
	cd $(WAILS_DIR) && $(WAILS) build -platform darwin/amd64 -clean -ldflags "$(LDFLAGS)"

build-windows-x64:
	cd $(WAILS_DIR) && $(WAILS) build -platform windows/amd64 -clean -ldflags "$(LDFLAGS)"

define package-mac-dmg
	@test -n "$(VERSION)" || (echo "VERSION is empty — check cmd/techcheck/internal/report/report.go ToolVersion"; exit 1)
	mkdir -p $(DIST)
	rm -f $(DIST)/$(NAME)-$(VERSION)-$(1).dmg
	create-dmg \
		--volname "$(NAME)" \
		--background "$(abspath $(INSTALL)/dmg-background.png)" \
		--window-pos 200 120 \
		--window-size $(DMG_WIN_W) $(DMG_WIN_H) \
		--icon-size $(DMG_ICON_PX) \
		--icon "$(NAME).app" 130 200 \
		--hide-extension "$(NAME).app" \
		--app-drop-link 410 200 \
		--add-file "Read Me First.txt" "$(abspath $(INSTALL)/README-macos.txt)" 270 80 \
		--no-internet-enable \
		$(DIST)/$(NAME)-$(VERSION)-$(1).dmg \
		$(BIN)/$(NAME).app
endef

sign-mac-arm64: build-mac-arm64
	codesign $(CODESIGN_FLAGS) --sign "$(SIGN_IDENTITY)" $(BIN)/$(NAME).app

sign-mac-x64: build-mac-x64
	codesign $(CODESIGN_FLAGS) --sign "$(SIGN_IDENTITY)" $(BIN)/$(NAME).app

# verify-sign-mac inspects whatever .app is currently in build/bin. Use after
# a sign-mac-* target. Catches missing entitlements, broken signature,
# Gatekeeper rejection.
verify-sign-mac:
	codesign --verify --deep --strict --verbose=2 $(BIN)/$(NAME).app
	codesign --display --entitlements - --xml $(BIN)/$(NAME).app | head -30
	@echo "--- spctl assessment ---"
	spctl --assess --type execute --verbose=4 $(BIN)/$(NAME).app || echo "(spctl rejection expected for ad-hoc; success expected for Developer ID)"

dist-mac-arm64: sign-mac-arm64
	$(call package-mac-dmg,arm64)

dist-mac-x64: sign-mac-x64
	$(call package-mac-dmg,x64)

# notarize-mac-* submits the packaged DMG to Apple's notary service, waits
# for the result, and staples the ticket onto the DMG on success. Skipped
# (warned, not failed) under ad-hoc signing. Requires APPLE_ID, APPLE_TEAM_ID,
# and APPLE_APP_PASSWORD (an app-specific password generated at
# appleid.apple.com → Sign-In and Security → App-Specific Passwords).
define notarize-dmg
	@if [ "$(SIGN_IDENTITY)" = "-" ]; then \
		echo "skip notarize-$(1): ad-hoc signing (APPLE_SIGN_IDENTITY unset)"; \
	else \
		test -n "$(APPLE_ID)"           || (echo "APPLE_ID not set";           exit 1); \
		test -n "$(APPLE_TEAM_ID)"      || (echo "APPLE_TEAM_ID not set";      exit 1); \
		test -n "$(APPLE_APP_PASSWORD)" || (echo "APPLE_APP_PASSWORD not set"; exit 1); \
		xcrun notarytool submit $(DIST)/$(NAME)-$(VERSION)-$(1).dmg \
			--apple-id "$(APPLE_ID)" --team-id "$(APPLE_TEAM_ID)" \
			--password "$(APPLE_APP_PASSWORD)" --wait; \
		xcrun stapler staple $(DIST)/$(NAME)-$(VERSION)-$(1).dmg; \
		xcrun stapler validate $(DIST)/$(NAME)-$(VERSION)-$(1).dmg; \
	fi
endef

notarize-mac-arm64: dist-mac-arm64
	$(call notarize-dmg,arm64)

notarize-mac-x64: dist-mac-x64
	$(call notarize-dmg,x64)

dist-windows-x64: build-windows-x64
	@test -n "$(VERSION)" || (echo "VERSION is empty — check cmd/techcheck/internal/report/report.go ToolVersion"; exit 1)
	mkdir -p $(DIST)
	cp $(BIN)/$(NAME).exe $(DIST)/$(NAME).exe
	cp $(INSTALL)/README-windows.txt "$(DIST)/Read Me First.txt"
	cd $(DIST) && zip -q "$(NAME)-$(VERSION)-x64.zip" "$(NAME).exe" "Read Me First.txt"
	rm -f $(DIST)/$(NAME).exe "$(DIST)/Read Me First.txt"

build-desktop: dist-mac-arm64 dist-mac-x64 dist-windows-x64
	@echo
	@echo "Desktop artifacts for $(VERSION) (signed identity: $(SIGN_IDENTITY)):"
	@ls -lh $(DIST)/

# release-desktop is build-desktop + Apple notarisation + ticket stapling.
# Fails fast if APPLE_SIGN_IDENTITY isn't set — there's no point notarising
# an ad-hoc signature, Apple will reject it.
release-desktop: notarize-mac-arm64 notarize-mac-x64 dist-windows-x64
	@echo
	@echo "Release artifacts for $(VERSION) (signed by: $(SIGN_IDENTITY)):"
	@ls -lh $(DIST)/
