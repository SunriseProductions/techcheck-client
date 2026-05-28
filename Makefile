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
  export UPLOAD_TOKEN INGEST_URL IT_CONTACT_EMAIL FALLBACK_POPS_JSON
endif

ifeq (,$(strip $(UPLOAD_TOKEN)$(INGEST_URL)$(IT_CONTACT_EMAIL)$(FALLBACK_POPS_JSON)))
  $(warning No build-time secrets supplied — building unauthenticated artifacts)
endif

LDFLAGS := -X '$(PKG)/cmd/techcheck/internal/config/defaults.UploadToken=$(UPLOAD_TOKEN)' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.IngestURL=$(INGEST_URL)' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.ITContactEmail=$(IT_CONTACT_EMAIL)' \
           -X '$(PKG)/cmd/techcheck/internal/config/defaults.FallbackPOPsJSON=$(FALLBACK_POPS_JSON)'

.PHONY: version test \
        build-mac-arm64 build-mac-x64 build-windows-x64 \
        dist-mac-arm64  dist-mac-x64  dist-windows-x64 \
        build-desktop clean-dist

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

dist-mac-arm64: build-mac-arm64
	$(call package-mac-dmg,arm64)

dist-mac-x64: build-mac-x64
	$(call package-mac-dmg,x64)

dist-windows-x64: build-windows-x64
	@test -n "$(VERSION)" || (echo "VERSION is empty — check cmd/techcheck/internal/report/report.go ToolVersion"; exit 1)
	mkdir -p $(DIST)
	cp $(BIN)/$(NAME).exe $(DIST)/$(NAME).exe
	cp $(INSTALL)/README-windows.txt "$(DIST)/Read Me First.txt"
	cd $(DIST) && zip -q "$(NAME)-$(VERSION)-x64.zip" "$(NAME).exe" "Read Me First.txt"
	rm -f $(DIST)/$(NAME).exe "$(DIST)/Read Me First.txt"

build-desktop: dist-mac-arm64 dist-mac-x64 dist-windows-x64
	@echo
	@echo "Desktop artifacts for $(VERSION):"
	@ls -lh $(DIST)/
