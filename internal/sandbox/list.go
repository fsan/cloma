package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Sandbox represents a sandbox instance.
type Sandbox struct {
	// Name is the unique identifier for the sandbox.
	Name string `json:"name"`

	// Agent is the agent type for the sandbox.
	Agent string `json:"agent"`

	// Status indicates the current state of the sandbox (e.g., "running", "stopped").
	Status string `json:"status"`

	// State is the field name newer CLI builds use for the same value as
	// Status. Either may be populated depending on the installed version.
	State string `json:"state"`

	// Image is the base image used for the sandbox.
	Image string `json:"image"`
}

// running reports whether the sandbox is running, accepting either of the
// field names the CLI versions use.
func (s Sandbox) running() bool {
	return s.Status == "running" || s.State == "running"
}

// sandboxListResponse represents the JSON response from `sbx ls --json` and
// the legacy `docker sandbox ls --json`.
//
// The standalone `sbx` CLI and newer plugin builds wrap the list under
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

// decodeSandboxList decodes one candidate JSON document, accepting either the
// object form ({"sandboxes": [...]}) or a bare array ([...]).
func decodeSandboxList(data []byte) ([]Sandbox, bool) {
	var array []Sandbox
	if err := json.Unmarshal(data, &array); err == nil {
		return array, true
	}

	var response sandboxListResponse
	if err := json.Unmarshal(data, &response); err == nil {
		return response.entries(), true
	}

	return nil, false
}

// parseSandboxList decodes the output of `sbx ls --json` (or the legacy
// `docker sandbox ls --json`).
//
// Some CLI versions print a daemon bootstrap banner (for example
// "Starting sandboxd daemon...") to stdout *before* the JSON document, so the
// raw output cannot be unmarshalled directly. We therefore try the whole
// output first, then fall back to the first line that looks like the start of a
// JSON document.
func parseSandboxList(output []byte) ([]Sandbox, error) {
	if sandboxes, ok := decodeSandboxList(output); ok {
		return sandboxes, nil
	}

	lines := bytes.Split(output, []byte("\n"))
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			continue
		}
		candidate := bytes.Join(lines[i:], []byte("\n"))
		if sandboxes, ok := decodeSandboxList(candidate); ok {
			return sandboxes, nil
		}
	}

	return nil, fmt.Errorf("failed to parse sandbox list: no JSON document found in output %q", strings.TrimSpace(string(output)))
}

// List returns all sandboxes managed by the installed CLI.
func List() ([]Sandbox, error) {
	cmd := Command("ls", "--json")
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
			return sb.running(), nil
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
