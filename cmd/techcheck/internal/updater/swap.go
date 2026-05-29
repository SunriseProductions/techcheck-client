package updater

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// RunApplyUpdateFromArgs parses the helper flags and runs the swap. Invoked
// from main.go when the binary is started with `--apply-update`.
func RunApplyUpdateFromArgs(args []string) error {
	fs := flag.NewFlagSet("apply-update", flag.ContinueOnError)
	pid := fs.Int("pid", 0, "parent pid to wait for")
	staged := fs.String("staged", "", "staged payload path")
	target := fs.String("target", "", "install target to replace")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pid == 0 || *staged == "" || *target == "" {
		return fmt.Errorf("apply-update: --pid, --staged and --target are required")
	}
	return RunApplyUpdate(*pid, *staged, *target)
}

// RunApplyUpdate waits for the parent to exit, swaps the staged payload into
// place, and relaunches. If the parent is still alive after the timeout it
// aborts rather than risk swapping a locked file.
func RunApplyUpdate(pid int, staged, target string) error {
	if err := waitForExit(pid, 30*time.Second); err != nil {
		return err
	}
	if err := applySwap(staged, target); err != nil {
		return err
	}
	return relaunch(target)
}

// waitForExit polls until the given pid is gone or the timeout elapses.
func waitForExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("updater: parent pid %d still alive after %s", pid, timeout)
}

// renameSwapDir replaces a non-empty directory target with staged using a
// two-step move (rename(2) cannot replace a non-empty dir). On any error
// before the second rename it restores the original. Used on macOS (.app) and
// as the non-Windows applySwap.
func renameSwapDir(staged, target string) error {
	old := target + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("updater: move current aside: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(old, target) // rollback
		return fmt.Errorf("updater: move new into place: %w", err)
	}
	_ = os.RemoveAll(old)
	return nil
}

// relaunch starts the freshly-installed app, detached from this helper.
func relaunch(target string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", target)
	} else {
		cmd = exec.Command(target)
	}
	return cmd.Start()
}
