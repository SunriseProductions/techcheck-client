//go:build !darwin

package updater

import (
	"path/filepath"
)

// prepareHelperFromExe copies the single executable into tmp and returns its
// path. Windows .exe and Linux binaries are self-contained — any code signature
// travels inside the file — so a lone copy runs fine. (macOS needs the whole
// .app bundle; see prepare_helper_darwin.go.)
func prepareHelperFromExe(self, tmp string) (string, error) {
	helper := filepath.Join(tmp, filepath.Base(self))
	if err := copyFile(self, helper, 0o755); err != nil {
		return "", err
	}
	return helper, nil
}
