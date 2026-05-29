package updater

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// SpawnHelper copies the current binary to a temp location and starts it
// detached with `--apply-update`, passing this process's pid so the helper can
// wait for us to exit before swapping. Returns once the helper has started.
func SpawnHelper(staged, target string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "techcheck-update-helper")
	if err != nil {
		return err
	}
	helper := filepath.Join(tmp, filepath.Base(self))
	if err := copyFile(self, helper, 0o755); err != nil {
		return err
	}

	cmd := exec.Command(helper,
		"--apply-update",
		"--pid", strconv.Itoa(os.Getpid()),
		"--staged", staged,
		"--target", target,
	)
	cmd.SysProcAttr = detachSysProcAttr()
	return cmd.Start()
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
