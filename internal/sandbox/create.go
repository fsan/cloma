package sandbox

import (
	"fmt"
	"os"
	"os/exec"
)

// Create creates a new sandbox with the given name for the specified workspace.
// If a warm template is available, it will be used to speed up sandbox creation.
// The sandbox is provisioned with the agent start script.
//
// When a sandbox with the given name already exists:
//   - If the recorded workspace matches the requested workspace, the existing
//     sandbox is reused as-is (no re-provisioning, no rebuild).
//   - If the workspace differs, the old sandbox is removed and a fresh one is
//     created so the new workspace path is mounted correctly.
func (c *SandboxClient) Create(sandboxName, workspace string) error {
	// Check if sandbox already exists
	exists, err := Exists(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check if sandbox exists: %w", err)
	}
	if exists {
		stored, err := GetStoredWorkspace(sandboxName)
		if err != nil {
			return fmt.Errorf("failed to read stored workspace: %w", err)
		}

		if stored == workspace {
			// The sandbox already exists with the same workspace; reuse it
			// as-is instead of rebuilding.
			return nil
		}

		// The workspace changed: remove the old sandbox so we can create a
		// fresh one with the new workspace mounted.
		fmt.Printf("Workspace changed (was %q, now %q); rebuilding sandbox %s\n", stored, workspace, sandboxName)
		if err := Clean(sandboxName); err != nil {
			return fmt.Errorf("failed to remove stale sandbox: %w", err)
		}
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

	// Record the workspace so a later run with the same name can detect
	// whether the sandbox should be reused or rebuilt.
	if err := StoreWorkspace(sandboxName, workspace); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not store workspace mapping: %v\n", err)
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
# CLOMA_AGENT selects which agent to install: "claude" (default), "grok",
# "kimi" or "openclaw".
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
    # Kimi's start script runs a small python3 relay to bridge Node fetch to
    # Ollama through cloma's non-tunneling proxy. Ensure python3 is present.
    if ! command -v python3 >/dev/null 2>&1; then
      apt-get update
      apt-get install -y --no-install-recommends python3
      rm -rf /var/lib/apt/lists/*
    fi
    ;;
  openclaw)
    # OpenClaw is a Node.js application and requires Node.js 22+. The base
    # sandbox image does not guarantee a new-enough Node, so install it via
    # NodeSource when missing or older than v22 before running OpenClaw's
    # installer. The agent's start script also relies on python3 for the
    # Ollama relay (shared with kimi), so ensure it is present too.
    need_node=0
    if ! command -v node >/dev/null 2>&1; then
      need_node=1
    else
      node_major="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
      if [ "${node_major}" -lt 22 ]; then
        need_node=1
      fi
    fi
    if [ "${need_node}" -eq 1 ]; then
      curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
      apt-get install -y --no-install-recommends nodejs
      rm -rf /var/lib/apt/lists/*
    fi
    if ! command -v openclaw >/dev/null 2>&1; then
      curl -fsSL https://openclaw.ai/install.sh | bash
    fi
    if ! command -v python3 >/dev/null 2>&1; then
      apt-get update
      apt-get install -y --no-install-recommends python3
      rm -rf /var/lib/apt/lists/*
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
