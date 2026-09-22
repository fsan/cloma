package sandbox

import (
	"fmt"
	"os/exec"
)

// allowHost registers host with the Docker sandbox network proxy, adding it
// to the sandbox's allow list. The verified form is "localhost:<port>" for
// host ports; external domains are passed through as-is (see allowDomain in
// policy.go for the fallback used when the proxy command does not accept
// them).
func (c *SandboxClient) allowHost(sandboxName, host string) error {
	cmd := exec.Command("docker", "sandbox", "network", "proxy", sandboxName,
		"--allow-host", host)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to configure network proxy: %w, output: %s", err, string(output))
	}

	return nil
}

// allowHostPort allows the sandbox to reach the given port on the host,
// reachable inside the sandbox as host.docker.internal:<port>.
func (c *SandboxClient) allowHostPort(sandboxName string, port int) error {
	return c.allowHost(sandboxName, fmt.Sprintf("localhost:%d", port))
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
