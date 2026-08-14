package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
	"github.com/spf13/cobra"
)

var shellWorkspace string
var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open an interactive shell in the sandbox",
	Long: `Open an interactive bash shell inside a Docker sandbox.

This command opens a bash shell in the sandbox for the specified workspace.
If the sandbox exists but is stopped, it will be started automatically.

The shell runs as the 'agent' user in the workspace directory.`,
	RunE: runShell,
}

func init() {
	rootCmd.AddCommand(shellCmd)

	shellCmd.Flags().StringVarP(&shellWorkspace, "workspace", "w", "", "Workspace directory (default: current directory)")
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "", "Sandbox name (overrides name generation from workspace)")
}

func runShell(cmd *cobra.Command, args []string) error {
	// Initialize config
	if err := config.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Resolve the sandbox name and its workspace together. When --name is
	// given without --workspace, the workspace is looked up from the
	// registry (recorded at sandbox creation) so the shell opens in the
	// right directory regardless of the caller's current working directory.
	// This matters for --tempfs runs, whose workspace is a host tmpfs path
	// the caller is not sitting in; without this, the shell defaults to cwd
	// and the exec fails to chdir into a path that doesn't exist in the sandbox.
	var sandboxName string
	var resolvedWorkspace string
	var err error

	if shellName != "" {
		sandboxName, err = workspace.ResolveSandboxName(shellName)
		if err != nil {
			return fmt.Errorf("invalid sandbox name %q: %w", shellName, err)
		}
		if shellWorkspace != "" {
			resolvedWorkspace, err = workspace.Resolve(shellWorkspace)
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w\nHint: Ensure the path exists: %s", err, shellWorkspace)
			}
		} else if stored, serr := sandbox.GetStoredWorkspace(sandboxName); serr == nil && stored != "" {
			// Use the workspace recorded when the sandbox was created.
			resolvedWorkspace = stored
		} else {
			resolvedWorkspace, err = workspace.Resolve(".")
			if err != nil {
				return fmt.Errorf("failed to resolve workspace: %w", err)
			}
		}
	} else {
		workspacePath := shellWorkspace
		if workspacePath == "" {
			workspacePath = "."
		}
		resolvedWorkspace, err = workspace.Resolve(workspacePath)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w\nHint: Ensure the path exists: %s", err, workspacePath)
		}
		// Generate sandbox name from the workspace path. If the path-derived
		// name has no registry entry, fall back to searching the registry for
		// a sandbox created with --name from this workspace.
		sandboxName = workspace.SandboxName(resolvedWorkspace)
		if stored, _ := sandbox.GetStoredWorkspace(sandboxName); stored == "" {
			if found, _ := sandbox.FindByWorkspace(resolvedWorkspace); found != "" {
				sandboxName = found
			}
		}
	}

	// Check prerequisites
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is not installed or not in PATH\nHint: Install Docker Desktop from https://www.docker.com/products/docker-desktop")
	}

	if err := sandbox.EnsureSandboxPlugin(); err != nil {
		return fmt.Errorf("Docker Desktop sandbox plugin required\nHint: Requires Docker Desktop 4.58+\nEnable sandbox plugin in Docker Desktop settings")
	}

	// Check if sandbox exists
	exists, err := sandbox.Exists(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check if sandbox exists: %w", err)
	}

	if !exists {
		return fmt.Errorf("sandbox does not exist: %s\nHint: Run 'cloma run' first to create the sandbox", sandboxName)
	}

	// Check if running, start if needed
	isRunning, err := sandbox.IsRunning(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check sandbox status: %w", err)
	}

	if !isRunning {
		if verbose > 0 {
			fmt.Printf("Starting sandbox: %s\n", sandboxName)
		}
		startCmd := exec.Command("docker", "sandbox", "start", sandboxName)
		startCmd.Stdout = os.Stdout
		startCmd.Stderr = os.Stderr
		if err := startCmd.Run(); err != nil {
			return fmt.Errorf("failed to start sandbox: %w", err)
		}
	}

	fmt.Printf("Opening shell in sandbox: %s\n", sandboxName)
	fmt.Printf("Workspace: %s\n\n", resolvedWorkspace)

	// Execute shell in sandbox
	return execShell(sandboxName, resolvedWorkspace)
}

func execShell(sandboxName, workspacePath string) error {
	args := []string{
		"sandbox", "exec",
		"-u", "agent",
		"-w", workspacePath,
		"-it",
		sandboxName,
		"bash",
	}

	dockerCmd := exec.Command("docker", args...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	return dockerCmd.Run()
}
