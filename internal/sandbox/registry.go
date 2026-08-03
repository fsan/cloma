package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sandboxRegistryDir is the subdirectory inside the cloma state directory
// used to store the workspace path associated with each sandbox. Each file is
// named after the sandbox and contains the absolute workspace path that the
// sandbox was created with.
const sandboxRegistryDir = "sandboxes"

// registryPath returns the path to the workspace file for a given sandbox name.
func registryPath(sandboxName string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	// State directory is ~/.cloma (see internal/config).
	stateDir := filepath.Join(home, ".cloma")
	return filepath.Join(stateDir, sandboxRegistryDir, sandboxName)
}

// StoreWorkspace records the workspace path associated with a sandbox name.
// This is used to detect whether a subsequent run with the same --name should
// reuse the existing sandbox or rebuild it because the workspace changed.
func StoreWorkspace(sandboxName, workspace string) error {
	dir := filepath.Dir(registryPath(sandboxName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create sandbox registry directory: %w", err)
	}
	path := registryPath(sandboxName)
	if err := os.WriteFile(path, []byte(workspace), 0644); err != nil {
		return fmt.Errorf("failed to store workspace for sandbox %s: %w", sandboxName, err)
	}
	return nil
}

// GetStoredWorkspace returns the workspace path recorded for a sandbox name.
// Returns "" and nil when no record exists.
func GetStoredWorkspace(sandboxName string) (string, error) {
	path := registryPath(sandboxName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read workspace for sandbox %s: %w", sandboxName, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// RemoveStoredWorkspace deletes the workspace record for a sandbox name.
// It is a no-op when the record does not exist.
func RemoveStoredWorkspace(sandboxName string) error {
	path := registryPath(sandboxName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove workspace record for sandbox %s: %w", sandboxName, err)
	}
	return nil
}

// FindByWorkspace searches the sandbox registry for a sandbox whose stored
// workspace path matches the given path. This is used as a fallback when a
// command derives a sandbox name from the workspace directory but no sandbox
// with that name exists — which happens when the sandbox was created with
// --name (a label-based name without a path hash).
//
// Returns the sandbox name and nil if a match is found, or "" and nil if no
// match exists.
func FindByWorkspace(workspace string) (string, error) {
	dir := filepath.Dir(registryPath("dummy"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read sandbox registry: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "cloma-") {
			continue
		}
		stored, err := GetStoredWorkspace(name)
		if err != nil {
			continue // skip unreadable entries
		}
		if stored == workspace {
			return name, nil
		}
	}
	return "", nil
}
