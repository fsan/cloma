package sandbox

import (
	"fmt"
	"os/exec"
)

// allowHost grants the sandbox access to a host or domain.
//
// With the standalone sbx CLI this is a sandbox-scoped network policy rule:
// `sbx policy allow network --sandbox <name> <resource>`, where the resource
// may carry a port suffix ("host.docker.internal:11434"). The legacy
// `docker sandbox` plugin instead configures its proxy directly with
// `--allow-host`.
func (c *SandboxClient) allowHost(sandboxName, host string) error {
	backend := DetectBackend()

	var cmd *exec.Cmd
	if backend.IsSBX() {
		cmd = backend.Command("policy", "allow", "network", "--sandbox", sandboxName, host)
	} else {
		cmd = backend.Command("network", "proxy", sandboxName, "--allow-host", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return CommandError("failed to configure network access", err, output)
	}

	return nil
}

// allowHostPort allows the sandbox to reach the given port on the host,
// reachable inside the sandbox as host.docker.internal:<port>.
//
// sbx policy rules match the hostname the connection names, which for cloma's
// model backend is host.docker.internal. The legacy proxy translates the
// sandbox-side host.docker.internal to localhost on the host, so it wants
// "localhost:<port>".
func (c *SandboxClient) allowHostPort(sandboxName string, port int) error {
	host := "localhost"
	if DetectBackend().IsSBX() {
		host = "host.docker.internal"
	}
	return c.allowHost(sandboxName, fmt.Sprintf("%s:%d", host, port))
}

// ConfigureProxy configures network proxy to allow the sandbox to reach host ports.
// This is used to allow the sandbox to connect to services like Ollama running on the host.
func (c *SandboxClient) ConfigureProxy(sandboxName string, port int) error {
	return c.allowHostPort(sandboxName, port)
}

// ConfigureProxyForOllama configures network proxy for the default Ollama port (11434).
func (c *SandboxClient) ConfigureProxyForOllama(sandboxName string) error {
	return c.ConfigureProxy(sandboxName, 11434)
}
