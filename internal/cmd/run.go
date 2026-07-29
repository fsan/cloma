package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/ollama"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	runWorkspace string
	runModel     string
	runPort      int
	runFlags     string
	runAgent     string
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
	cmd.Flags().StringVar(&runAgent, "agent", "", "Code agent to run: claude (default), grok (Grok Build) or kimi (Kimi Code)")
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

	// Resolve the code agent (claude by default, grok for Grok Build, kimi for Kimi Code)
	agent := runAgent
	if agent == "" {
		agent = config.GetAgent()
	}
	agent = sandbox.NormalizeAgent(agent)

	// Resolve workspace
	workspacePath := runWorkspace
	if workspacePath == "" {
		workspacePath = "."
	}

	resolvedWorkspace, err := workspace.Resolve(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace: %w\nHint: Ensure the path exists: %s", err, workspacePath)
	}

	// Generate sandbox name
	sandboxName := workspace.SandboxName(resolvedWorkspace)

	// Display configuration
	if verbose > 0 {
		fmt.Println("=== Launching Agent in Sandbox ===")
		fmt.Println()
	}
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("Agent: %s\n", agent)
	fmt.Printf("Ollama: http://host.docker.internal:%d\n", ollamaPort)
	fmt.Printf("Workspace: %s\n", resolvedWorkspace)
	fmt.Printf("Sandbox: %s\n", sandboxName)
	if runFlags != "" {
		fmt.Printf("Flags: %s\n", runFlags)
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

	// Execute agent in sandbox
	return launchAgent(sandboxName, resolvedWorkspace, envVars)
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
