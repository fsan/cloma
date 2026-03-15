package sandbox

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		// Check if provisioned
		provisioned, err := c.isProvisioned(sandboxName)
		if err != nil {
			return fmt.Errorf("failed to check if sandbox is provisioned: %w", err)
		}
		if !provisioned {
			if err := c.provisionSandbox(sandboxName); err != nil {
				return fmt.Errorf("failed to provision existing sandbox: %w", err)
			}
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

// isProvisioned checks if the sandbox has the start script installed.
func (c *SandboxClient) isProvisioned(sandboxName string) (bool, error) {
	cmd := exec.Command("docker", "sandbox", "exec", sandboxName,
		"test", "-x", "/usr/local/bin/start-agent.sh")
	return cmd.Run() == nil, nil
}

// provisionSandbox installs the agent start script into the sandbox.
func (c *SandboxClient) provisionSandbox(sandboxName string) error {
	// Find the script file - check relative paths
	scriptPath := c.StartScriptPath

	// Try relative to current directory first
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Try relative to executable directory
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			altPath := filepath.Join(execDir, scriptPath)
			if _, err := os.Stat(altPath); err == nil {
				scriptPath = altPath
			}
		}
	}

	// Read and base64-encode the start script
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read start script from %s: %w", scriptPath, err)
	}
	scriptB64 := base64.StdEncoding.EncodeToString(scriptContent)

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

# Install agent CLI if not present
if ! command -v claude >/dev/null 2>&1; then
	curl -fsSL https://claude.ai/install.sh | bash
fi

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