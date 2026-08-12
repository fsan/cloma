package sandbox

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fsan/cloma/internal/workspace"
)

// Remove removes a sandbox completely.
// This deletes the sandbox and all its data, including the stored workspace
// mapping used to decide reuse vs. rebuild on subsequent runs.
//
// If the sandbox was created with --tempfs, the associated tmpfs mount on the
// host is unmounted and its mount point removed (best-effort).
func Remove(sandboxName string) error {
	// Capture the recorded workspace before the registry entry is removed so
	// we can clean up a tmpfs mount if the sandbox used one.
	stored, _ := GetStoredWorkspace(sandboxName)

	cmd := exec.Command("docker", "sandbox", "rm", sandboxName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove sandbox %s: %w, output: %s", sandboxName, err, string(output))
	}
	// Clean up the workspace registry entry so a later create does not
	// think the sandbox still exists with the old workspace.
	_ = RemoveStoredWorkspace(sandboxName)

	// Best-effort tmpfs cleanup; a sandbox created with --tempfs keeps its
	// in-memory mount on the host until the sandbox is removed.
	if stored != "" {
		if uerr := workspace.UnmountTmpfs(stored); uerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not unmount tmpfs workspace %s: %v\n", stored, uerr)
		}
	}
	return nil
}

// RemoveIfExists removes a sandbox if it exists.
// Returns nil if the sandbox doesn't exist.
func RemoveIfExists(sandboxName string) error {
	exists, err := Exists(sandboxName)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	return Remove(sandboxName)
}

// Clean removes the sandbox completely (stop + remove).
// This is a convenience function that stops and removes the sandbox.
func Clean(sandboxName string) error {
	// Stop first (ignoring errors if already stopped)
	_ = Stop(sandboxName)

	// Then remove
	return Remove(sandboxName)
}
