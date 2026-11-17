// Package sandbox provides sandbox management functionality for cloma.
// It wraps the standalone `sbx` CLI, falling back to the legacy
// `docker sandbox` plugin, for creating, managing, and interacting with
// sandbox containers.
package sandbox

import (
	_ "embed"
	"encoding/base64"
	"errors"
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
	AgentPi       = "pi"
	AgentDsh      = "dsh"
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
	// "kimi" (Kimi Code), "openclaw" (OpenClaw), "junie" (Junie CLI),
	// "pi" (Pi coding agent) and "dsh" (DeepSeek Harness).
	Agent string

	// OllamaPort is the host port the sandbox must be able to reach for its
	// model backend. It is registered with the sandbox network proxy before the
	// agent is provisioned, so the agent's install (npm/curl) can reach the
	// internet through that proxy — the sandbox has no direct egress, so
	// without this the install fails with ECONNREFUSED to the proxy.
	// Default: 11434.
	OllamaPort int
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

// WithOllamaPort sets the host port registered with the sandbox network proxy
// before provisioning so the agent install can reach the internet.
func WithOllamaPort(port int) Option {
	return func(c *SandboxClient) {
		c.OllamaPort = port
	}
}

// NewClient creates a new SandboxClient with default configuration.
// Options can be passed to customize the client.
func NewClient(opts ...Option) *SandboxClient {
	c := &SandboxClient{
		TemplateTag:  "claude-code-sandbox-template:warm",
		AgentVersion: "latest",
		Agent:        AgentClaude,
		OllamaPort:   11434,
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
	case AgentPi:
		return AgentPi
	case AgentDsh:
		return AgentDsh
	default:
		return AgentClaude
	}
}

// StartScriptBase64 returns the embedded start script as base64-encoded string.
func (c *SandboxClient) StartScriptBase64() string {
	return base64.StdEncoding.EncodeToString(startScript)
}

// ErrSandboxCLINotAvailable is returned when neither the standalone `sbx` CLI
// nor the legacy `docker sandbox` plugin is available.
var ErrSandboxCLINotAvailable = errors.New("sandbox CLI not available (install sbx: https://docs.docker.com/ai/sandboxes/)")

// EnsureSandboxCLI verifies that a usable sandbox CLI is available: the
// standalone `sbx` CLI when installed, otherwise the legacy `docker sandbox`
// plugin. It returns ErrSandboxCLINotAvailable when neither works.
func EnsureSandboxCLI() error {
	if err := DetectBackend().Command("version").Run(); err != nil {
		return ErrSandboxCLINotAvailable
	}
	return nil
}

// SandboxKind reports which sandbox backend cloma will drive ("sbx" or
// "docker"), for display in diagnostics.
func SandboxKind() string {
	if DetectBackend().IsSBX() {
		return "sbx"
	}
	return "docker sandbox"
}
