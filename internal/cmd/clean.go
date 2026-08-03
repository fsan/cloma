package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
	"github.com/spf13/cobra"
)

var cleanWorkspace string
var cleanName string
var cleanForce bool

// cleanCmd represents the clean command
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove a sandbox completely",
	Long: `Remove a Docker sandbox completely.

This command stops and removes the sandbox for the specified workspace.
All data in the sandbox will be lost, but the workspace directory on the host
is preserved.

Use --name (or a positional argument) to remove a sandbox by name, bypassing
name generation from the workspace path. The value is treated as a label:
cloma slugifies it and ensures the "cloma-" prefix, so passing either a label
(e.g. "instance1") or a full name copied from 'cloma list' (e.g.
"cloma-instance1") works. When --name is not provided, the sandbox name is
derived from the workspace directory (default: current directory).

Use --force to skip the confirmation prompt.`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().StringVarP(&cleanWorkspace, "workspace", "w", "", "Workspace directory (default: current directory)")
	cleanCmd.Flags().StringVarP(&cleanName, "name", "n", "", "Sandbox name (overrides name generation from workspace)")
	cleanCmd.Flags().BoolVarP(&cleanForce, "force", "f", false, "Skip confirmation prompt")
}

func runClean(cmd *cobra.Command, args []string) error {
	// Initialize config
	if err := config.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Determine the sandbox name: use a positional argument if provided,
	// then --name, otherwise derive it from the workspace path (default:
	// current directory).
	var sandboxName string
	var resolvedWorkspace string

	if len(args) > 0 && args[0] != "" {
		name, err := workspace.ResolveSandboxName(args[0])
		if err != nil {
			return fmt.Errorf("invalid sandbox name %q: %w", args[0], err)
		}
		sandboxName = name
		resolvedWorkspace = "<by name>"
	} else if cleanName != "" {
		name, err := workspace.ResolveSandboxName(cleanName)
		if err != nil {
			return fmt.Errorf("invalid sandbox name %q: %w", cleanName, err)
		}
		sandboxName = name
		resolvedWorkspace = "<by name>"
	} else {
		// Derive sandbox name from the workspace path (default: current
		// directory), with a fallback to the registry for sandboxes created
		// with --name.
		var err error
		sandboxName, resolvedWorkspace, err = resolveSandboxNameFromWorkspace(cleanWorkspace)
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

	// Confirm removal unless --force is set
	if !cleanForce {
		fmt.Printf("This will remove sandbox: %s\n", sandboxName)
		fmt.Printf("Workspace: %s\n", resolvedWorkspace)
		fmt.Println("All data in the sandbox will be lost.")
		fmt.Print("Continue? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Printf("Removing sandbox: %s\n", sandboxName)

	// Stop first if running
	isRunning, err := sandbox.IsRunning(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check sandbox status: %w", err)
	}

	if isRunning {
		fmt.Println("Stopping sandbox first...")
		if err := sandbox.Stop(sandboxName); err != nil {
			// Non-fatal, continue with removal
			fmt.Fprintf(os.Stderr, "Warning: failed to stop sandbox: %v\n", err)
		}
	}

	// Remove the sandbox
	if err := sandbox.Remove(sandboxName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not remove sandbox completely: %v\n", err)
		return err
	}

	fmt.Println("Sandbox removed.")
	return nil
}
