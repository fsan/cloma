package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// TmpfsSubDir is the subdirectory inside the cloma state directory used to
// hold --tempfs mount points. Each sandbox using --tempfs on Linux (with
// privileges) gets a tmpfs mount named after the sandbox, e.g.
// ~/.cloma/tmpfs/cloma-myproject-a1b2c3d4.
const TmpfsSubDir = "tmpfs"

// tempDirBase is the parent directory for the plain-directory fallback used
// when a real tmpfs mount is unavailable (e.g. macOS, or Linux without the
// privileges needed to mount). cloma creates an empty directory named
// cloma-<sandbox> here and uses it as the workspace. It defaults to /tmp.
var tempDirBase = "/tmp"

// attemptTmpfsMount controls whether CreateTmpfs tries a real tmpfs mount on
// Linux before falling back to the /tmp directory. It defaults to true and is
// only toggled by tests to force the fallback path deterministically.
var attemptTmpfsMount = true

// TmpfsBaseDir returns the directory under the cloma state dir where real
// tmpfs mount points live (Linux only).
func TmpfsBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".cloma", TmpfsSubDir)
}

// IsTmpfsWorkspace reports whether path is a cloma-managed --tempfs
// workspace: either a real tmpfs mount under ~/.cloma/tmpfs, or a plain
// fallback directory directly under the temp dir (cloma-<sandbox>).
func IsTmpfsWorkspace(path string) bool {
	if path == "" {
		return false
	}
	if isChildOf(TmpfsBaseDir(), path) {
		return true
	}
	if isChildOf(tempDirBase, path) && strings.HasPrefix(filepath.Base(path), "cloma-") {
		return true
	}
	return false
}

// CreateTmpfs creates an ephemeral workspace for the given sandbox name and
// returns its absolute path along with whether it is a real in-memory tmpfs
// mount.
//
// On Linux it first tries to mount a real tmpfs (size is a mount option such
// as "1g" or "512m"; empty falls back to the kernel default, typically half
// the RAM). If a tmpfs mount is unavailable — e.g. on macOS, where the kernel
// has no tmpfs, or on Linux without root/passwordless sudo — it falls back to
// a plain empty directory under the temp dir (/tmp) so --tempfs still works
// cross-platform without privileges.
func CreateTmpfs(sandboxName, size string) (string, bool, error) {
	if attemptTmpfsMount && runtime.GOOS == "linux" {
		if path, err := createTmpfsMount(sandboxName, size); err == nil {
			return path, true, nil
		}
		// Mount failed (no privileges or restricted environment); fall through
		// to the plain-directory fallback below.
	}

	// sandboxName already carries the "cloma-" prefix (it comes from
	// RandomSandboxName or ResolveSandboxName), so use it directly to match
	// the tmpfs-mount branch and avoid a double "cloma-cloma-" prefix.
	dir := filepath.Join(tempDirBase, sandboxName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create temp workspace at %s: %w", dir, err)
	}
	return dir, false, nil
}

// createTmpfsMount creates a real tmpfs mount point for the sandbox and
// returns its path. It is Linux-only and requires root or passwordless sudo.
func createTmpfsMount(sandboxName, size string) (string, error) {
	base := TmpfsBaseDir()
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("failed to create tmpfs base directory: %w", err)
	}

	dir := filepath.Join(base, sandboxName)
	// MkdirAll so re-running with an existing (possibly already-mounted) point
	// does not fail.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create tmpfs mount point: %w", err)
	}

	if isMountPoint(dir) {
		// Already mounted (e.g. reused from a previous run); reuse as-is.
		return dir, nil
	}

	if err := mountTmpfs(dir, size); err != nil {
		// Best effort: remove the empty mount point we just created so the
		// state dir does not accumulate empty directories.
		_ = os.Remove(dir)
		return "", err
	}
	return dir, nil
}

// UnmountTmpfs cleans up a cloma-managed --tempfs workspace. For a real tmpfs
// mount it unmounts first; for the plain-directory fallback it just removes
// the directory. It is a no-op when path is not a --tempfs workspace.
func UnmountTmpfs(path string) error {
	if !IsTmpfsWorkspace(path) {
		return nil
	}

	if isMountPoint(path) {
		if err := umount(path); err != nil {
			return fmt.Errorf("failed to unmount tmpfs %s: %w", path, err)
		}
	}

	// Remove the (now empty) mount point / fallback directory.
	_ = os.RemoveAll(path)
	return nil
}

// isChildOf reports whether path is a direct child of base (a single path
// segment below base, without escaping it).
func isChildOf(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !strings.ContainsRune(rel, filepath.Separator)
}

// mountTmpfs mounts a tmpfs at dir with the given size option.
func mountTmpfs(dir, size string) error {
	opts := "mode=0755"
	if size != "" {
		opts = "size=" + size + "," + opts
	}
	return runPrivileged("mount", "-t", "tmpfs", "-o", opts, "tmpfs", dir)
}

// umount unmounts the filesystem at dir.
func umount(dir string) error {
	return runPrivileged("umount", dir)
}

// runPrivileged runs a mount/umount command directly when running as root,
// or via `sudo -n` when not. It returns the command's run error.
func runPrivileged(name string, args ...string) error {
	if os.Geteuid() == 0 {
		return exec.Command(name, args...).Run()
	}
	full := append([]string{"-n", name}, args...)
	return exec.Command("sudo", full...).Run()
}

// isMountPoint reports whether dir is currently a mount point. It prefers the
// `mountpoint` utility when available, falling back to parsing /proc/mounts
// (Linux).
func isMountPoint(dir string) bool {
	if _, err := exec.LookPath("mountpoint"); err == nil {
		c := exec.Command("mountpoint", "-q", dir)
		if c.Run() == nil {
			return true
		}
		// mountpoint exists but returned non-zero; fall through to /proc/mounts
		// in case mountpoint is present but the dir is a bind mount it doesn't
		// recognize.
	}

	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == dir {
			return true
		}
	}
	return false
}
