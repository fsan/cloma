package cmd

import (
	"fmt"
	"os/exec"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
	"github.com/spf13/cobra"
)

var stopWorkspace string
var stopName string

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop [name]",
	Short: "Stop a running sandbox",
	Long: `Stop a running Docker sandbox.

This command stops the sandbox for the specified workspace.
The sandbox is preserved and can be restarted later with 'cloma run' or 'cloma shell'.

Use a positional argument or --name (or -n) to stop a sandbox by name, bypassing
name generation from the workspace path. The value is treated as a label:
cloma slugifies it and ensures the "cloma-" prefix, so passing either a label
(e.g. "instance1") or a full name copied from 'cloma list' (e.g.
"cloma-instance1") works. When no name is provided, the sandbox name is
derived from the workspace directory (default: current directory).

If the sandbox does not exist or is already stopped, this command does nothing.`,
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)

	stopCmd.Flags().StringVarP(&stopWorkspace, "workspace", "w", "", "Workspace directory (default: current directory)")
	stopCmd.Flags().StringVarP(&stopName, "name", "n", "", "Sandbox name (overrides name generation from workspace)")
}

func runStop(cmd *cobra.Command, args []string) error {
	// Initialize config
	if err := config.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Determine the sandbox name: use a positional argument if provided,
	// then --name, otherwise derive it from the workspace path (default:
	// current directory).
	var sandboxName string
	if len(args) > 0 && args[0] != "" {
		name, err := workspace.ResolveSandboxName(args[0])
		if err != nil {
			return fmt.Errorf("invalid sandbox name %q: %w", args[0], err)
		}
		sandboxName = name
	} else if stopName != "" {
		var err error
		sandboxName, err = workspace.ResolveSandboxName(stopName)
		if err != nil {
			return fmt.Errorf("invalid sandbox name %q: %w", stopName, err)
		}
	} else {
		// Derive sandbox name from the workspace path (default: current
		// directory), with a fallback to the registry for sandboxes created
		// with --name.
		var err error
		sandboxName, _, err = resolveSandboxNameFromWorkspace(stopWorkspace)
		if err != nil {
			return err
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
		fmt.Printf("Sandbox does not exist: %s\n", sandboxName)
		return nil
	}

	// Check if running
	isRunning, err := sandbox.IsRunning(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check sandbox status: %w", err)
	}

	if !isRunning {
		fmt.Printf("Sandbox is not running: %s\n", sandboxName)
		return nil
	}

	// Stop the sandbox
	fmt.Printf("Stopping sandbox: %s\n", sandboxName)
	if err := sandbox.Stop(sandboxName); err != nil {
		return fmt.Errorf("failed to stop sandbox: %w", err)
	}

	fmt.Println("Sandbox stopped.")
	return nil
}
