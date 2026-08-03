package sandbox

import (
	"fmt"
	"os/exec"
)

// Remove removes a sandbox completely.
// This deletes the sandbox and all its data, including the stored workspace
// mapping used to decide reuse vs. rebuild on subsequent runs.
func Remove(sandboxName string) error {
	cmd := exec.Command("docker", "sandbox", "rm", sandboxName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove sandbox %s: %w, output: %s", sandboxName, err, string(output))
	}
	// Clean up the workspace registry entry so a later create does not
	// think the sandbox still exists with the old workspace.
	_ = RemoveStoredWorkspace(sandboxName)
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
