package updater

import (
	"archive/zip"
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

// Stage extracts the update payload from artifactPath into targetDir, next to
// where the swap will place it (same directory -> same volume -> rename is
// atomic-ish). Returns the staged path. Any pre-existing staged path is
// removed first.
func Stage(artifactPath, targetDir string) (string, error) {
	if runtime.GOOS == "darwin" {
		staged := filepath.Join(targetDir, stagedAppMac)
		_ = os.RemoveAll(staged)
		if err := extractDMGApp(artifactPath, staged); err != nil {
			return "", err
		}
		return staged, nil
	}
	staged := filepath.Join(targetDir, stagedExeWin)
	_ = os.Remove(staged)
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
		defer out.Close()
		_, err = io.Copy(out, io.LimitReader(rc, hardCap))
		return err
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
		return fmt.Errorf("updater: hdiutil attach: %v: %s", err, out)
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
		return fmt.Errorf("updater: ditto: %v: %s", err, out)
	}
	return nil
}
