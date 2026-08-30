package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jsonMarshal is a small wrapper around json.Marshal so the registry can encode
// metadata without importing encoding/json at every call site.
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal is a small wrapper around json.Unmarshal.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

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

// sandboxMeta is the JSON envelope stored in each registry file. It records
// the workspace path, the agent type, and the creation time for a sandbox.
//
// For backward compatibility, older registry files contain only the workspace
// path as plain text. GetMetadata detects this and treats the whole file
// contents as the workspace path with an empty agent and a zero creation time
// (falling back to the file's modification time).
type sandboxMeta struct {
	Workspace string    `json:"workspace"`
	Agent     string    `json:"agent,omitempty"`
	Created   time.Time `json:"created,omitempty"`
}

// StoreWorkspace records the workspace path associated with a sandbox name.
// This is used to detect whether a subsequent run with the same --name should
// reuse the existing sandbox or rebuild it because the workspace changed.
//
// The agent and creation time are also recorded so the list/UI can group by
// agent and sort by creation. When the sandbox already exists with the same
// workspace, the existing metadata (agent/created) is preserved.
func StoreWorkspace(sandboxName, workspace string) error {
	return StoreMetadata(sandboxName, workspace, "")
}

// StoreMetadata records the workspace path, agent, and creation time for a
// sandbox. When agent is empty and a record already exists, the existing agent
// is preserved (so a reuse-run that doesn't pass the agent doesn't wipe it).
// The creation time is set to now only when creating a new record; on reuse it
// is preserved.
func StoreMetadata(sandboxName, workspace, agent string) error {
	dir := filepath.Dir(registryPath(sandboxName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create sandbox registry directory: %w", err)
	}

	meta := sandboxMeta{Workspace: workspace, Agent: agent, Created: time.Now()}

	// Preserve existing agent/created when reusing an existing record and the
	// caller did not supply a new agent.
	if existing, err := readMeta(sandboxName); err == nil && existing.Workspace == workspace {
		if agent == "" {
			meta.Agent = existing.Agent
		}
		if !existing.Created.IsZero() {
			meta.Created = existing.Created
		}
	}

	data, err := jsonMarshal(meta)
	if err != nil {
		return fmt.Errorf("failed to encode metadata for sandbox %s: %w", sandboxName, err)
	}
	path := registryPath(sandboxName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to store workspace for sandbox %s: %w", sandboxName, err)
	}
	return nil
}

// GetStoredWorkspace returns the workspace path recorded for a sandbox name.
// Returns "" and nil when no record exists.
func GetStoredWorkspace(sandboxName string) (string, error) {
	meta, err := GetMetadata(sandboxName)
	if err != nil {
		return "", err
	}
	return meta.Workspace, nil
}

// GetStoredAgent returns the agent recorded for a sandbox name.
// Returns "" and nil when no record exists or the agent was not recorded.
func GetStoredAgent(sandboxName string) (string, error) {
	meta, err := GetMetadata(sandboxName)
	if err != nil {
		return "", err
	}
	return meta.Agent, nil
}

// GetCreationTime returns the creation time recorded for a sandbox name.
// When the recorded time is zero (e.g. an old plain-text registry entry), it
// falls back to the registry file's modification time. Returns the zero time
// and nil when no record exists.
func GetCreationTime(sandboxName string) (time.Time, error) {
	meta, err := GetMetadata(sandboxName)
	if err != nil {
		return time.Time{}, err
	}
	if !meta.Created.IsZero() {
		return meta.Created, nil
	}
	// Fall back to the file modification time for legacy entries.
	info, err := os.Stat(registryPath(sandboxName))
	if err != nil {
		return time.Time{}, nil
	}
	return info.ModTime(), nil
}

// GetMetadata returns the full metadata record for a sandbox name.
func GetMetadata(sandboxName string) (sandboxMeta, error) {
	data, err := os.ReadFile(registryPath(sandboxName))
	if err != nil {
		if os.IsNotExist(err) {
			return sandboxMeta{}, nil
		}
		return sandboxMeta{}, fmt.Errorf("failed to read metadata for sandbox %s: %w", sandboxName, err)
	}
	return readMetaBytes(data)
}

// readMeta reads the metadata for a sandbox name without returning errors for
// missing files (returns an empty meta).
func readMeta(sandboxName string) (sandboxMeta, error) {
	data, err := os.ReadFile(registryPath(sandboxName))
	if err != nil {
		return sandboxMeta{}, err
	}
	return readMetaBytes(data)
}

// readMetaBytes parses a registry file. It first tries JSON; if that fails it
// treats the contents as a legacy plain-text workspace path.
func readMetaBytes(data []byte) (sandboxMeta, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var meta sandboxMeta
		if err := jsonUnmarshal(trimmed, &meta); err == nil {
			return meta, nil
		}
	}
	// Legacy plain-text format: the whole file is the workspace path.
	return sandboxMeta{Workspace: strings.TrimSpace(string(data))}, nil
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
