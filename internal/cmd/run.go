package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/ollama"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	runWorkspace     string
	runModel         string
	runPort          int
	runFlags         string
	runAgent         string
	runName          string
	runEnv           []string
	runTempfs        bool
	runTempfsSize    string
	runTempfsMounted bool // set during run when --tempfs produced a real tmpfs mount
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an agent in a Docker sandbox",
	Long: `Run an agent in an isolated Docker sandbox.

This command will:
  1. Resolve the workspace path
  2. Check prerequisites (Ollama, model, Docker sandbox plugin)
  3. Create the sandbox if needed
  4. Configure network proxy for host access
  5. Launch the agent interactively

The sandbox is isolated from your host system but has access to the
specified workspace directory and can connect to Ollama running on the host.`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Register the run flags on both the `run` subcommand and the root command.
	// They share the same backing variables, so `cloma run --agent grok` and
	// `cloma --agent grok` (no subcommand) behave identically. The root command
	// defaults to running an agent, matching the documented quick-start form.
	addRunFlags(runCmd)
	addRunFlags(rootCmd)
	rootCmd.RunE = runRun

	viper.BindPFlag("model", runCmd.Flags().Lookup("model"))
	viper.BindPFlag("agent", runCmd.Flags().Lookup("agent"))
}

// addRunFlags binds the run command's flags to the shared package variables
// on the given command. Safe to call on multiple commands because each
// command owns an independent flag set.
func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&runWorkspace, "workspace", "w", "", "Workspace directory (default: current directory)")
	cmd.Flags().StringVarP(&runModel, "model", "m", "", "AI model to use (default: glm-5:cloud)")
	cmd.Flags().IntVarP(&runPort, "port", "p", 0, "Ollama port (default: 11434)")
	cmd.Flags().StringVarP(&runFlags, "flags", "f", "", "Additional flags to pass to the agent")
	cmd.Flags().StringVar(&runAgent, "agent", "", "Code agent to run: claude (default), grok (Grok Build), kimi (Kimi Code) or openclaw (OpenClaw)")
	cmd.Flags().StringVarP(&runName, "name", "n", "", "Name this cloma instance (overrides the workspace-derived sandbox name, enabling multiple instances from the same folder)")
	// --env can be repeated to inject multiple environment variables into the
	// sandbox. Each value must be in KEY=VALUE form, e.g. --env 'DEBUG=1'.
	cmd.Flags().StringArrayVarP(&runEnv, "env", "e", nil, "Environment variable to set in the sandbox (KEY=VALUE); repeatable")
	// --tempfs replaces the local directory with an ephemeral workspace so the
	// sandbox never touches the real filesystem. On Linux (with root or
	// passwordless sudo) it mounts a real in-memory tmpfs; on macOS or without
	// privileges it falls back to a plain empty directory under /tmp.
	cmd.Flags().BoolVar(&runTempfs, "tempfs", false, "Use an ephemeral in-memory (tmpfs) workspace on the host instead of the local directory (falls back to a /tmp dir on macOS)")
	cmd.Flags().StringVar(&runTempfsSize, "tempfs-size", "1g", "Size of the tmpfs workspace (e.g. 1g, 512m); used with --tempfs on Linux")
}

func runRun(cmd *cobra.Command, args []string) error {
	// Initialize config
	if err := config.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Get configuration values
	model := runModel
	if model == "" {
		model = config.GetModel()
	}

	ollamaPort := runPort
	if ollamaPort == 0 {
		ollamaPort = config.GetOllamaPort()
	}

	ollamaURL := fmt.Sprintf("http://localhost:%d", ollamaPort)

	// Resolve the code agent (claude by default, grok for Grok Build, kimi for
	// Kimi Code, openclaw for OpenClaw).
	agent := runAgent
	if agent == "" {
		agent = config.GetAgent()
	}
	agent = sandbox.NormalizeAgent(agent)

	// Validate user-supplied --env entries before doing any work so a
	// malformed value fails fast instead of after sandbox provisioning.
	for _, e := range runEnv {
		if !isValidEnvAssignment(e) {
			return fmt.Errorf("invalid --env value %q: expected KEY=VALUE form (e.g. --env 'DEBUG=1')", e)
		}
	}

	// Resolve the workspace and derive the sandbox name. With --tempfs the
	// workspace is an in-memory (tmpfs) mount on the host instead of a local
	// directory; the sandbox name must be known first so the mount point can
	// be named after it.
	var resolvedWorkspace string
	var sandboxName string
	var err error
	if runTempfs {
		if runName != "" {
			sandboxName, err = workspace.ResolveSandboxName(runName)
			if err != nil {
				return fmt.Errorf("invalid sandbox name %q: %w", runName, err)
			}
		} else {
			sandboxName, err = workspace.RandomSandboxName()
			if err != nil {
				return fmt.Errorf("failed to generate sandbox name: %w", err)
			}
		}
		var isTmpfsMount bool
		resolvedWorkspace, isTmpfsMount, err = workspace.CreateTmpfs(sandboxName, runTempfsSize)
		if err != nil {
			return fmt.Errorf("failed to create tmpfs workspace: %w", err)
		}
		// Stash whether we got a real tmpfs mount so the display line below
		// can distinguish it from the /tmp fallback used on macOS.
		runTempfsMounted = isTmpfsMount
	} else {
		workspacePath := runWorkspace
		if workspacePath == "" {
			if runName != "" {
				// A named instance is meant to run from a real folder, so default
				// to the current directory instead of creating a random workspace.
				// This lets several named instances share the same workspace.
				workspacePath = "."
			}
			// else: leave empty so Resolve creates a random workspace.
		}

		resolvedWorkspace, err = workspace.Resolve(workspacePath)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace: %w\nHint: Ensure the path exists: %s", err, workspacePath)
		}

		// Generate sandbox name: use the user-supplied label (--name) when
		// given, otherwise derive it from the workspace path. The label lets
		// the user run several instances from the same folder without name
		// collisions.
		if runName != "" {
			sandboxName, err = workspace.ResolveSandboxName(runName)
			if err != nil {
				return fmt.Errorf("invalid sandbox name %q: %w", runName, err)
			}
		} else {
			sandboxName = workspace.SandboxName(resolvedWorkspace)
		}
	}

	// Display configuration
	if verbose > 0 {
		fmt.Println("=== Launching Agent in Sandbox ===")
		fmt.Println()
	}
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Agent: %s\n", agent)
	fmt.Printf("Ollama: http://host.docker.internal:%d\n", ollamaPort)
	if runTempfs {
		if runTempfsMounted {
			fmt.Printf("Workspace: %s (tmpfs, %s)\n", resolvedWorkspace, runTempfsSize)
		} else {
			// Fallback: a plain empty directory under /tmp (e.g. on macOS,
			// where a kernel tmpfs mount is not available).
			fmt.Printf("Workspace: %s (temp dir)\n", resolvedWorkspace)
		}
	} else {
		fmt.Printf("Workspace: %s\n", resolvedWorkspace)
	}
	fmt.Printf("Sandbox: %s\n", sandboxName)
	if runFlags != "" {
		fmt.Printf("Flags: %s\n", runFlags)
	}
	if len(runEnv) > 0 {
		fmt.Printf("Env: %s\n", strings.Join(runEnv, " "))
	}
	fmt.Println()

	// Check prerequisites
	if verbose > 0 {
		fmt.Println("Checking prerequisites...")
	}

	// Check Docker
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is not installed or not in PATH\nHint: Install Docker Desktop from https://www.docker.com/products/docker-desktop")
	}

	// Check sandbox plugin
	if err := sandbox.EnsureSandboxPlugin(); err != nil {
		return fmt.Errorf("Docker Desktop sandbox plugin required\nHint: Requires Docker Desktop 4.58+\nEnable sandbox plugin in Docker Desktop settings")
	}

	// Create Ollama client and check availability
	ollamaClient := ollama.NewClient(ollamaURL)
	if err := ollamaClient.WaitForAvailable(20); err != nil {
		return err
	}

	// Ensure model exists
	if err := ollamaClient.EnsureModel(model); err != nil {
		return err
	}

	// Create sandbox client
	sandboxClient := sandbox.NewClient(
		sandbox.WithTemplateTag(config.GetTemplateTag()),
		sandbox.WithAgent(agent),
	)

	// Ensure sandbox exists
	if verbose > 0 {
		fmt.Println("Ensuring sandbox exists...")
	}
	if err := sandboxClient.Create(sandboxName, resolvedWorkspace); err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Check if sandbox is running
	isRunning, err := sandbox.IsRunning(sandboxName)
	if err != nil {
		return fmt.Errorf("failed to check sandbox status: %w", err)
	}

	if verbose > 0 {
		if isRunning {
			fmt.Printf("Sandbox is running: %s\n", sandboxName)
		} else {
			fmt.Printf("Sandbox exists but not running, will be started on exec: %s\n", sandboxName)
		}
	}

	// Configure network proxy for host access
	if verbose > 0 {
		fmt.Printf("Configuring network proxy for host port %d...\n", ollamaPort)
	}
	if err := sandboxClient.ConfigureProxy(sandboxName, ollamaPort); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not configure network proxy.\n")
		fmt.Fprintf(os.Stderr, "Sandbox may not be able to reach Ollama on host.\n")
	}

	// Launch agent
	fmt.Println("Launching agent...")
	fmt.Println()

	// Build environment variables for the start script.
	// The script reads these generic CLOMA_* values for all agents and
	// derives agent-specific configuration (e.g. ANTHROPIC_* for Claude Code,
	// ~/.grok/config.toml for Grok Build, ~/.kimi-code/config.toml for Kimi
	// Code) from them.
	sandboxOllamaURL := fmt.Sprintf("http://host.docker.internal:%d", ollamaPort)
	envVars := []string{
		fmt.Sprintf("CLOMA_AGENT=%s", agent),
		fmt.Sprintf("CLOMA_MODEL=%s", model),
		fmt.Sprintf("CLOMA_OLLAMA_URL=%s", sandboxOllamaURL),
	}
	// Claude Code also consumes the Anthropic-style variables directly; keep
	// passing them so the default agent behaves exactly as before.
	if agent == sandbox.AgentClaude {
		envVars = append(envVars,
			"ANTHROPIC_AUTH_TOKEN=ollama",
			"ANTHROPIC_API_KEY=",
			fmt.Sprintf("ANTHROPIC_BASE_URL=%s", sandboxOllamaURL),
			fmt.Sprintf("CLAUDE_CODE_MODEL=%s", model),
		)
	}
	if runFlags != "" {
		envVars = append(envVars, fmt.Sprintf("CLOMA_FLAGS=%s", runFlags))
		// Backwards-compatible alias used by the previous Claude Code script.
		if agent == sandbox.AgentClaude {
			envVars = append(envVars, fmt.Sprintf("CLAUDE_CODE_FLAGS=%s", runFlags))
		}
	}

	// Append user-supplied --env entries last so they can override any of the
	// cloma-managed variables above (e.g. CLOMA_MODEL) if the user wants to.
	envVars = append(envVars, runEnv...)

	// Execute agent in sandbox
	return launchAgent(sandboxName, resolvedWorkspace, envVars)
}

// isValidEnvAssignment reports whether s is a valid KEY=VALUE assignment
// suitable for passing through to the sandbox with `docker exec -e`.
// The key must be a non-empty shell-style identifier and may not contain a
// space; the value may be empty.
func isValidEnvAssignment(s string) bool {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return false
	}
	key := s[:eq]
	if key == "" || strings.ContainsAny(key, " \t") {
		return false
	}
	// First character must be a letter or underscore to match shell env
	// variable naming conventions.
	c := key[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
		return false
	}
	return true
}

func launchAgent(sandboxName, workspacePath string, envVars []string) error {
	args := []string{
		"sandbox", "exec",
		"-it",
		"-u", "agent",
		"-w", workspacePath,
	}

	for _, env := range envVars {
		args = append(args, "-e", env)
	}

	args = append(args, sandboxName, "/usr/local/bin/start-agent.sh")

	dockerCmd := exec.Command("docker", args...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr

	return dockerCmd.Run()
}
