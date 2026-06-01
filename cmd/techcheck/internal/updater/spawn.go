package updater

import (
	"io"
	"os"
	"os/exec"
	"strconv"
)

// SpawnHelper prepares a runnable copy of this app in a temp location and
// starts it detached with `--apply-update`, passing this process's pid so the
// helper can wait for us to exit before swapping. Returns once the helper has
// started.
func SpawnHelper(staged, target string) error {
	tmp, err := os.MkdirTemp("", "techcheck-update-helper")
	if err != nil {
		return err
	}
	helper, err := prepareHelper(tmp)
	if err != nil {
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

// prepareHelper copies the running binary into tmp in a form the OS will
// execute and returns the path to run. The copy mechanism is platform-specific
// (see prepareHelperFromExe): on macOS the whole signed .app bundle is copied,
// because a lone Mach-O lifted out of the bundle has an invalid signature and is
// killed under the hardened runtime.
func prepareHelper(tmp string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return prepareHelperFromExe(self, tmp)
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
