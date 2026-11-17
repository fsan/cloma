package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// ExecInteractive runs an interactive command in the sandbox.
// It attaches stdin, stdout, and stderr to the current process.
// Typically used for running agents interactively.
func (c *SandboxClient) ExecInteractive(sandboxName, workspace string, cmd ...string) error {
	args := []string{"exec", "-it", "-u", "agent", "-w", workspace, sandboxName}
	args = append(args, cmd...)

	dockerCmd := Command(args...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	// Set up signal handling for interactive sessions
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the command
	if err := dockerCmd.Start(); err != nil {
		return fmt.Errorf("failed to start interactive command: %w", err)
	}

	// Handle signals by forwarding to the process
	go func() {
		<-sigChan
		if dockerCmd.Process != nil {
			dockerCmd.Process.Signal(syscall.SIGINT)
		}
	}()

	// Wait for completion
	if err := dockerCmd.Wait(); err != nil {
		return fmt.Errorf("interactive command failed: %w", err)
	}

	return nil
}

// Exec runs a non-interactive command in the sandbox and returns the output.
// This is useful for running commands and capturing their output.
func Exec(sandboxName string, cmd ...string) (string, error) {
	args := []string{"exec", sandboxName}
	args = append(args, cmd...)

	dockerCmd := Command(args...)

	var stdout, stderr bytes.Buffer
	dockerCmd.Stdout = &stdout
	dockerCmd.Stderr = &stderr

	if err := dockerCmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecWithPrivilege runs a privileged command in the sandbox as root.
// This is useful for provisioning operations.
func ExecWithPrivilege(sandboxName string, cmd ...string) (string, error) {
	// sbx has no --privileged flag, so it only passes the root user there.
	backend := DetectBackend()
	args := []string{"exec"}
	if !backend.IsSBX() {
		args = append(args, "--privileged")
	}
	args = append(args, "-u", "root", sandboxName)
	args = append(args, cmd...)

	dockerCmd := backend.Command(args...)

	var stdout, stderr bytes.Buffer
	dockerCmd.Stdout = &stdout
	dockerCmd.Stderr = &stderr

	if err := dockerCmd.Run(); err != nil {
		return "", fmt.Errorf("privileged command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
