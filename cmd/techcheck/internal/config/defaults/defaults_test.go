package defaults_test

import (
	"testing"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/defaults"
)

func TestPackageVarsExistAndDefaultEmpty(t *testing.T) {
	// These variables are populated at build time via -ldflags. In a `go test`
	// run with no -ldflags, they must default to empty strings so the package
	// is safe to import in any environment.
	if defaults.UploadToken != "" {
		t.Errorf("UploadToken should default to empty, got %q", defaults.UploadToken)
	}
	if defaults.IngestURL != "" {
		t.Errorf("IngestURL should default to empty, got %q", defaults.IngestURL)
	}
	if defaults.ITContactEmail != "" {
		t.Errorf("ITContactEmail should default to empty, got %q", defaults.ITContactEmail)
	}
	if defaults.FallbackPOPsJSON != "" {
		t.Errorf("FallbackPOPsJSON should default to empty, got %q", defaults.FallbackPOPsJSON)
	}
}
