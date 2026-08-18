// Package sandbox provides Docker Desktop sandbox management functionality for cloma.
// It wraps the `docker sandbox` CLI commands for creating, managing, and interacting
// with Docker Desktop sandbox containers.
package sandbox

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"os/exec"
)

//go:embed start-agent.sh
var startScript []byte

// Supported code agents that can be launched inside a sandbox.
const (
	AgentClaude   = "claude"
	AgentGrok     = "grok"
	AgentKimi     = "kimi"
	AgentOpenClaw = "openclaw"
	AgentJunie    = "junie"
)

// SandboxClient holds configuration for sandbox operations.
type SandboxClient struct {
	// TemplateTag is the Docker image tag for the warm template.
	// Default: "cloma-sandbox-template:warm"
	TemplateTag string

	// AgentVersion is the version of the agent to install.
	// Default: "latest"
	AgentVersion string

	// Agent is the code agent to launch inside the sandbox.
	// Supported values: "claude" (Claude Code, default), "grok" (Grok Build),
	// "kimi" (Kimi Code), "openclaw" (OpenClaw) and "junie" (Junie CLI).
	Agent string
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

// WithAgent sets the code agent to launch inside the sandbox.
// The value is normalized via NormalizeAgent.
func WithAgent(agent string) Option {
	return func(c *SandboxClient) {
		c.Agent = NormalizeAgent(agent)
	}
}

// NewClient creates a new SandboxClient with default configuration.
// Options can be passed to customize the client.
func NewClient(opts ...Option) *SandboxClient {
	c := &SandboxClient{
		TemplateTag:  "claude-code-sandbox-template:warm",
		AgentVersion: "latest",
		Agent:        AgentClaude,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NormalizeAgent validates and normalizes an agent name.
// Unknown values fall back to the default agent (Claude Code).
func NormalizeAgent(agent string) string {
	switch agent {
	case "", AgentClaude:
		return AgentClaude
	case AgentGrok:
		return AgentGrok
	case AgentKimi:
		return AgentKimi
	case AgentOpenClaw:
		return AgentOpenClaw
	case AgentJunie:
		return AgentJunie
	default:
		return AgentClaude
	}
}

// StartScriptBase64 returns the embedded start script as base64-encoded string.
func (c *SandboxClient) StartScriptBase64() string {
	return base64.StdEncoding.EncodeToString(startScript)
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
