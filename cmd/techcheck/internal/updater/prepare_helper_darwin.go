//go:build darwin

package updater

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
)

// prepareHelperFromExe copies the whole .app bundle (not just the Mach-O) into
// tmp and returns the path to the executable inside the copy.
//
// A Developer ID + hardened-runtime app is signed across the entire bundle
// (Info.plist + _CodeSignature/CodeResources), and the embedded Mach-O signature
// references those sibling resources. A lone executable lifted out of the bundle
// therefore fails signature validation and is SIGKILLed by AMFI the instant it
// launches — which is why the in-place updater silently never ran on macOS while
// working on Windows (a self-contained .exe). ditto preserves the signature,
// xattrs, and symlinks, so the copied bundle launches cleanly.
func prepareHelperFromExe(self, tmp string) (string, error) {
	bundle, err := resolveTarget("darwin", self)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(tmp, filepath.Base(bundle))
	if out, err := exec.Command("ditto", bundle, dst).CombinedOutput(); err != nil {
		return "", fmt.Errorf("updater: copy helper bundle: %w: %s", err, bytes.TrimSpace(out))
	}
	rel, err := filepath.Rel(bundle, self)
	if err != nil {
		return "", err
	}
	return filepath.Join(dst, rel), nil
}
