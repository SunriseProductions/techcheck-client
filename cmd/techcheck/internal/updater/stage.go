package updater

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	appName      = "SunriseTechCheck"
	stagedAppMac = "." + appName + ".app.new"
	stagedExeWin = "." + appName + ".exe.new"
)

// stagedPath returns the staging sibling next to the install target's dir.
func stagedPath(targetDir string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(targetDir, stagedAppMac)
	}
	return filepath.Join(targetDir, stagedExeWin)
}

// Stage extracts the update payload from artifactPath into targetDir, next to
// where the swap will place it (same directory -> same volume -> rename is
// atomic-ish). Returns the staged path. Any pre-existing staged path is
// removed first.
func Stage(artifactPath, targetDir string) (string, error) {
	staged := stagedPath(targetDir)
	if runtime.GOOS == "darwin" {
		_ = os.RemoveAll(staged) // .app is a directory bundle
		if err := extractDMGApp(artifactPath, staged); err != nil {
			return "", err
		}
		return staged, nil
	}
	_ = os.Remove(staged) // single .exe file
	if err := extractZipExe(artifactPath, staged); err != nil {
		return "", err
	}
	return staged, nil
}

// extractZipExe copies the SunriseTechCheck.exe entry out of a release zip to
// dest. Pure Go — runs on any platform.
func extractZipExe(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.Base(f.Name) != appName+".exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(rc, hardCap)); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close() // capture flush error — a corrupt staged exe must not look OK
	}
	return fmt.Errorf("updater: %s.exe not found in zip", appName)
}

// extractDMGApp mounts a notarised DMG, copies the .app out with ditto
// (preserving the code signature), and detaches. darwin-only at runtime.
func extractDMGApp(dmgPath, dest string) error {
	mount, err := os.MkdirTemp("", "techcheck-dmg")
	if err != nil {
		return err
	}
	attach := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, dmgPath)
	if out, err := attach.CombinedOutput(); err != nil {
		return fmt.Errorf("updater: hdiutil attach: %w: %s", err, bytes.TrimSpace(out))
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mount, "-force").Run()
		_ = os.RemoveAll(mount)
	}()

	src := filepath.Join(mount, appName+".app")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("updater: %s.app not found in dmg: %w", appName, err)
	}
	if out, err := exec.Command("ditto", src, dest).CombinedOutput(); err != nil {
		return fmt.Errorf("updater: ditto: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}
