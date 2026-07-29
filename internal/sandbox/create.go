package sandbox

import (
	"fmt"
	"os"
	"os/exec"
)

// Create creates a new sandbox with the given name for the specified workspace.
// If a warm template is available, it will be used to speed up sandbox creation.
// The sandbox is provisioned with the agent start script.
func (c *SandboxClient) Create(sandboxName, workspace string) error {
	// Check if sandbox already exists
	exists, err := Exists(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check if sandbox exists: %w", err)
	}
	if exists {
		// Always (re)provision an existing sandbox so the embedded start
		// script and agent CLI match the currently running cloma binary.
		// The sandbox may have been created/provisioned by an older binary
		// whose start script lacks support for newer agents (e.g. kimi);
		// skipping re-provisioning would launch that stale script and fail
		// with "unknown agent". Re-provisioning is idempotent and cheap.
		if err := c.provisionSandbox(sandboxName); err != nil {
			return fmt.Errorf("failed to provision existing sandbox: %w", err)
		}
		return nil
	}

	// Create the sandbox
	var cmd *exec.Cmd
	if c.templateExists() {
		cmd = exec.Command("docker", "sandbox", "create",
			"--name", sandboxName,
			"--load-local-template",
			"-t", c.TemplateTag,
			"claude",
			workspace,
		)
	} else {
		cmd = exec.Command("docker", "sandbox", "create",
			"--name", sandboxName,
			"claude",
			workspace,
		)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Provision with start script
	if err := c.provisionSandbox(sandboxName); err != nil {
		return fmt.Errorf("failed to provision sandbox: %w", err)
	}

	return nil
}

// templateExists checks if the template image exists.
func (c *SandboxClient) templateExists() bool {
	cmd := exec.Command("docker", "image", "inspect", c.TemplateTag)
	return cmd.Run() == nil
}

// provisionSandbox installs the agent start script into the sandbox.
func (c *SandboxClient) provisionSandbox(sandboxName string) error {
	// Use embedded start script (base64-encoded)
	scriptB64 := c.StartScriptBase64()

	// Run provision script inside the sandbox
	provisionScript := `
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# Install dependencies if needed
if ! command -v curl >/dev/null 2>&1 || ! command -v git >/dev/null 2>&1; then
	apt-get update
	apt-get install -y --no-install-recommends curl ca-certificates git
	rm -rf /var/lib/apt/lists/*
fi

# Install the requested agent CLI if not present.
# CLOMA_AGENT selects which agent to install: "claude" (default), "grok" or "kimi".
CLOMA_AGENT="${CLOMA_AGENT:-claude}"
case "$CLOMA_AGENT" in
  grok)
    if ! command -v grok >/dev/null 2>&1; then
      curl -fsSL https://x.ai/cli/install.sh | bash
    fi
    ;;
  kimi)
    if ! command -v kimi >/dev/null 2>&1; then
      curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash
    fi
    ;;
  claude|*)
    if ! command -v claude >/dev/null 2>&1; then
      curl -fsSL https://claude.ai/install.sh | bash
    fi
    ;;
esac

# Ensure start script directory exists
install -d -m 0755 /usr/local/bin

# Decode and install start script
printf "%s" "$SCRIPT_B64" | base64 -d > /usr/local/bin/start-agent.sh
chmod 0755 /usr/local/bin/start-agent.sh
chown agent:agent /usr/local/bin/start-agent.sh 2>/dev/null || true

printf "Start script installed successfully\n"
`

	cmd := exec.Command("docker", "sandbox", "exec",
		"--privileged",
		"-u", "root",
		"-e", "CLOMA_AGENT_VERSION="+c.AgentVersion,
		"-e", "CLOMA_AGENT="+c.Agent,
		"-e", "SCRIPT_B64="+scriptB64,
		sandboxName,
		"bash", "-lc", provisionScript,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to provision sandbox: %w", err)
	}

	return nil
}
