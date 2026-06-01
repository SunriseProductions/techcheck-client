//go:build darwin

package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// A Developer ID + hardened-runtime .app is signed across the whole bundle
// (Info.plist + _CodeSignature/CodeResources), not just the Mach-O. If the
// updater helper copies only the executable out of the bundle, its signature is
// invalid and AMFI SIGKILLs it at launch — so the swap never runs. The helper
// must therefore copy the entire bundle.
func TestPrepareHelperFromExeCopiesWholeBundleOnDarwin(t *testing.T) {
	src := t.TempDir()
	app := filepath.Join(src, "Foo.app")
	macos := filepath.Join(app, "Contents", "MacOS")
	csdir := filepath.Join(app, "Contents", "_CodeSignature")
	for _, d := range []string{macos, csdir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(macos, "Foo")
	mustWrite(t, exe, "MACHO", 0o755)
	mustWrite(t, filepath.Join(app, "Contents", "Info.plist"), "<plist/>", 0o644)
	mustWrite(t, filepath.Join(csdir, "CodeResources"), "<resources/>", 0o644)

	dstTmp := t.TempDir()
	helper, err := prepareHelperFromExe(exe, dstTmp)
	if err != nil {
		t.Fatal(err)
	}

	// The helper is the executable inside a copied bundle, not a lone file.
	want := filepath.Join(dstTmp, "Foo.app", "Contents", "MacOS", "Foo")
	if helper != want {
		t.Errorf("helper = %q; want %q", helper, want)
	}
	// The whole bundle must be present — the signature spans these files.
	for _, rel := range []string{
		"Contents/MacOS/Foo",
		"Contents/Info.plist",
		"Contents/_CodeSignature/CodeResources",
	} {
		if _, err := os.Stat(filepath.Join(dstTmp, "Foo.app", rel)); err != nil {
			t.Errorf("copied bundle missing %s: %v", rel, err)
		}
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
