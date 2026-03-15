// Package sandbox provides Docker Desktop sandbox management functionality for cloma.
// It wraps the `docker sandbox` CLI commands for creating, managing, and interacting
// with Docker Desktop sandbox containers.
package sandbox

import (
	"errors"
	"os/exec"
)

// SandboxClient holds configuration for sandbox operations.
type SandboxClient struct {
	// TemplateTag is the Docker image tag for the warm template.
	// Default: "cloma-sandbox-template:warm"
	TemplateTag string

	// AgentVersion is the version of the agent to install.
	// Default: "latest"
	AgentVersion string

	// StartScriptPath is the path to the start-claude-code.sh script.
	// Default: "./image/start-claude-code.sh" (relative to project root)
	StartScriptPath string
}

// Option is a function that configures a SandboxClient.
type Option func(*SandboxClient)

// WithTemplateTag sets the template tag for the sandbox client.
func WithTemplateTag(tag string) Option {
	return func(c *SandboxClient) {
		c.TemplateTag = tag
	}
}

// WithAgentVersion sets the agent version for the sandbox client.
func WithAgentVersion(version string) Option {
	return func(c *SandboxClient) {
		c.AgentVersion = version
	}
}

// WithStartScriptPath sets the path to the start script.
func WithStartScriptPath(path string) Option {
	return func(c *SandboxClient) {
		c.StartScriptPath = path
	}
}

// NewClient creates a new SandboxClient with default configuration.
// Options can be passed to customize the client.
func NewClient(opts ...Option) *SandboxClient {
	c := &SandboxClient{
		TemplateTag:     "claude-code-sandbox-template:warm",
		AgentVersion:    "latest",
		StartScriptPath: "./image/start-claude-code.sh",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ErrSandboxPluginNotAvailable is returned when the Docker sandbox plugin is not installed.
var ErrSandboxPluginNotAvailable = errors.New("docker sandbox plugin not available")

// EnsureSandboxPlugin verifies that the Docker Desktop sandbox plugin is available.
// It returns ErrSandboxPluginNotAvailable if the plugin is not installed or not working.
func EnsureSandboxPlugin() error {
	cmd := exec.Command("docker", "sandbox", "version")
	if err := cmd.Run(); err != nil {
		return ErrSandboxPluginNotAvailable
	}
	return nil
}