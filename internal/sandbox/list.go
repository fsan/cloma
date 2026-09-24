package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Sandbox represents a Docker Desktop sandbox instance.
type Sandbox struct {
	// Name is the unique identifier for the sandbox.
	Name string `json:"name"`

	// Agent is the agent type for the sandbox.
	Agent string `json:"agent"`

	// Status indicates the current state of the sandbox (e.g., "running", "stopped").
	Status string `json:"status"`

	// Image is the base image used for the sandbox.
	Image string `json:"image"`
}

// sandboxListResponse represents the JSON response from `docker sandbox ls --json`.
//
// Newer CLI builds (and the standalone `sbx` successor) wrap the list under
// "sandboxes"; older Docker Desktop builds used "vms". Both are accepted.
type sandboxListResponse struct {
	Sandboxes []Sandbox `json:"sandboxes"`
	VMs       []Sandbox `json:"vms"`
}

// entries returns the sandbox list regardless of which key carried it.
func (r sandboxListResponse) entries() []Sandbox {
	if r.Sandboxes != nil {
		return r.Sandboxes
	}
	return r.VMs
}

// parseSandboxList decodes the output of `docker sandbox ls --json`.
//
// Some CLI versions print a daemon bootstrap banner (for example
// "Starting sandboxd daemon...") to stdout *before* the JSON document, so the
// raw output cannot be unmarshalled directly. We therefore try the whole
// output first, then fall back to the first line that looks like the start of a
// JSON document.
func parseSandboxList(output []byte) ([]Sandbox, error) {
	var response sandboxListResponse
	if err := json.Unmarshal(output, &response); err == nil {
		return response.entries(), nil
	}

	lines := bytes.Split(output, []byte("\n"))
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			continue
		}
		candidate := bytes.Join(lines[i:], []byte("\n"))
		if err := json.Unmarshal(candidate, &response); err == nil {
			return response.entries(), nil
		}
	}

	return nil, fmt.Errorf("failed to parse sandbox list: no JSON document found in output %q", strings.TrimSpace(string(output)))
}

// List returns all Docker Desktop sandboxes.
// It parses the JSON output from `docker sandbox ls --json`.
func List() ([]Sandbox, error) {
	cmd := exec.Command("docker", "sandbox", "ls", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list sandboxes: %w", err)
	}

	return parseSandboxList(output)
}

// Exists checks if a sandbox with the given name exists.
func Exists(name string) (bool, error) {
	sandboxes, err := List()
	if err != nil {
		return false, err
	}

	for _, sb := range sandboxes {
		if sb.Name == name {
			return true, nil
		}
	}

	return false, nil
}

// IsRunning checks if a sandbox with the given name is currently running.
func IsRunning(name string) (bool, error) {
	sandboxes, err := List()
	if err != nil {
		return false, err
	}

	for _, sb := range sandboxes {
		if sb.Name == name {
			return sb.Status == "running", nil
		}
	}

	return false, nil
}

// Get retrieves a specific sandbox by name.
// Returns nil if the sandbox doesn't exist.
func Get(name string) (*Sandbox, error) {
	sandboxes, err := List()
	if err != nil {
		return nil, err
	}

	for _, sb := range sandboxes {
		if sb.Name == name {
			return &sb, nil
		}
	}

	return nil, nil
}
