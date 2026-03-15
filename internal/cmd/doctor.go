package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/ollama"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
)

// doctorCmd represents the doctor command
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on the system",
	Long: `Run health checks to validate the Docker sandbox setup.

This command checks:
  1. Docker installation
  2. Docker Desktop sandbox plugin
  3. Ollama connectivity
  4. Model availability in Ollama
  5. Workspace directory
  6. Warm template availability
  7. Sandbox status

Exit codes:
  0: All checks passed or only warnings
  1: One or more checks failed`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // OK, FAIL, WARN
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// DoctorOutput holds all check results
type DoctorOutput struct {
	Checks  []CheckResult `json:"checks"`
	Summary Summary       `json:"summary"`
}

// Summary holds the summary counts
type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Initialize config
	if err := config.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Get configuration
	model := config.GetModel()
	ollamaURL := config.GetOllamaURL()
	templateTag := config.GetTemplateTag()

	// Resolve workspace (use current directory if not specified)
	workspacePath := "."
	resolvedWorkspace, err := workspace.Resolve(workspacePath)
	if err != nil {
		resolvedWorkspace = ""
	}

	// Generate sandbox name
	sandboxName := ""
	if resolvedWorkspace != "" {
		sandboxName = workspace.SandboxName(resolvedWorkspace)
	}

	var checks []CheckResult
	var errors, warnings int

	if jsonOutput {
		// JSON output mode
		checks = runChecksJSON(model, ollamaURL, templateTag, resolvedWorkspace, sandboxName, &errors, &warnings)
		output := DoctorOutput{
			Checks: checks,
			Summary: Summary{
				Errors:   errors,
				Warnings: warnings,
			},
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			return err
		}
	} else {
		// Text output mode
		fmt.Println("=== Cloma Docker Doctor ===")
		fmt.Println()
		checks = runChecksText(model, ollamaURL, templateTag, resolvedWorkspace, sandboxName, &errors, &warnings)

		// Print summary
		fmt.Println()
		fmt.Println("=== Summary ===")

		if errors == 0 && warnings == 0 {
			printGreen("All checks passed!")
			fmt.Println("Ready to run: cloma run")
		} else if errors == 0 {
			printYellow(fmt.Sprintf("%d warning(s), 0 error(s)", warnings))
			fmt.Println("Setup is functional but could be improved.")
		} else {
			printRed(fmt.Sprintf("%d error(s), %d warning(s)", errors, warnings))
			fmt.Println("Please fix the errors above before running.")
		}
	}

	if errors > 0 {
		os.Exit(1)
	}
	return nil
}

func runChecksJSON(model, ollamaURL, templateTag, resolvedWorkspace, sandboxName string, errors, warnings *int) []CheckResult {
	var checks []CheckResult

	// Check 1: Docker
	check := checkDocker()
	checks = append(checks, check)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 2: Sandbox plugin
	check = checkSandboxPlugin()
	checks = append(checks, check)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 3: Ollama
	check = checkOllama(ollamaURL)
	checks = append(checks, check)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 4: Model
	check = checkModel(ollamaURL, model)
	checks = append(checks, check)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 5: Workspace
	check = checkWorkspace(resolvedWorkspace)
	checks = append(checks, check)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 6: Template
	check = checkTemplate(templateTag)
	checks = append(checks, check)
	if check.Status == "WARN" {
		*warnings++
	}

	// Check 7: Sandbox
	check = checkSandbox(sandboxName)
	checks = append(checks, check)

	return checks
}

func runChecksText(model, ollamaURL, templateTag, resolvedWorkspace, sandboxName string, errors, warnings *int) []CheckResult {
	var checks []CheckResult

	// Check 1: Docker
	fmt.Print("Checking Docker installation... ")
	check := checkDocker()
	checks = append(checks, check)
	printCheckResult(check, false)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 2: Sandbox plugin
	fmt.Print("Checking Docker Desktop sandbox plugin... ")
	check = checkSandboxPlugin()
	checks = append(checks, check)
	printCheckResult(check, false)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 3: Ollama
	fmt.Print("Checking Ollama connectivity... ")
	check = checkOllama(ollamaURL)
	checks = append(checks, check)
	printCheckResult(check, false)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 4: Model
	fmt.Printf("Checking model %s... ", model)
	check = checkModel(ollamaURL, model)
	checks = append(checks, check)
	printCheckResult(check, false)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 5: Workspace
	fmt.Print("Checking workspace directory... ")
	check = checkWorkspace(resolvedWorkspace)
	checks = append(checks, check)
	printCheckResult(check, true)
	if check.Status == "FAIL" {
		*errors++
	}

	// Check 6: Template
	fmt.Print("Checking warm template... ")
	check = checkTemplate(templateTag)
	checks = append(checks, check)
	printCheckResult(check, true)
	if check.Status == "WARN" {
		*warnings++
	}

	// Check 7: Sandbox
	fmt.Print("Checking sandbox... ")
	check = checkSandbox(sandboxName)
	checks = append(checks, check)
	printCheckResult(check, true)

	return checks
}

func printCheckResult(check CheckResult, showDetail bool) {
	switch check.Status {
	case "OK":
		printGreen("OK")
		if showDetail && check.Message != "" {
			fmt.Printf("  %s\n", check.Message)
		}
	case "WARN":
		printYellow("WARN")
		if check.Message != "" {
			fmt.Printf("  %s\n", check.Message)
		}
		if check.Hint != "" {
			fmt.Printf("  %s\n", check.Hint)
		}
	case "FAIL":
		printRed("FAIL")
		if check.Message != "" {
			fmt.Printf("  %s\n", check.Message)
		}
		if check.Hint != "" {
			fmt.Printf("  %s\n", check.Hint)
		}
	}
}

func checkDocker() CheckResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return CheckResult{
			Name:    "Docker",
			Status:  "FAIL",
			Message: "Docker is not installed or not in PATH",
			Hint:    "Install Docker Desktop from https://www.docker.com/products/docker-desktop",
		}
	}

	// Get Docker version
	output, err := exec.Command("docker", "--version").Output()
	if err != nil {
		return CheckResult{
			Name:    "Docker",
			Status:  "FAIL",
			Message: "Failed to get Docker version",
		}
	}

	return CheckResult{
		Name:    "Docker",
		Status:  "OK",
		Message: strings.TrimSpace(string(output)),
	}
}

func checkSandboxPlugin() CheckResult {
	if err := sandbox.EnsureSandboxPlugin(); err != nil {
		return CheckResult{
			Name:    "Sandbox Plugin",
			Status:  "FAIL",
			Message: "Docker Desktop sandbox plugin is required",
			Hint:    "Requires Docker Desktop 4.58+. Enable sandbox plugin in Docker Desktop settings.",
		}
	}

	return CheckResult{
		Name:   "Sandbox Plugin",
		Status: "OK",
	}
}

func checkOllama(ollamaURL string) CheckResult {
	client := ollama.NewClient(ollamaURL)
	if !client.IsAvailable() {
		return CheckResult{
			Name:    "Ollama",
			Status:  "FAIL",
			Message: fmt.Sprintf("Cannot reach Ollama at %s", ollamaURL),
			Hint:    "Ensure Ollama is running: ollama serve",
		}
	}

	return CheckResult{
		Name:    "Ollama",
		Status:  "OK",
		Message: ollamaURL,
	}
}

func checkModel(ollamaURL, model string) CheckResult {
	client := ollama.NewClient(ollamaURL)
	if !client.ModelExists(model) {
		return CheckResult{
			Name:    "Model",
			Status:  "FAIL",
			Message: fmt.Sprintf("Model %s not found in Ollama", model),
			Hint:    fmt.Sprintf("Pull it first: ollama pull %s", model),
		}
	}

	return CheckResult{
		Name:   "Model",
		Status: "OK",
	}
}

func checkWorkspace(workspace string) CheckResult {
	if workspace == "" {
		return CheckResult{
			Name:    "Workspace",
			Status:  "FAIL",
			Message: "Workspace directory does not exist",
		}
	}

	return CheckResult{
		Name:    "Workspace",
		Status:  "OK",
		Message: workspace,
	}
}

func checkTemplate(templateTag string) CheckResult {
	// Check if template image exists
	cmd := exec.Command("docker", "image", "inspect", templateTag)
	if cmd.Run() != nil {
		return CheckResult{
			Name:    "Template",
			Status:  "WARN",
			Message: fmt.Sprintf("Warm template not found: %s", templateTag),
			Hint:    "First run will be slower. Warm templates are optional.",
		}
	}

	return CheckResult{
		Name:    "Template",
		Status:  "OK",
		Message: templateTag,
	}
}

func checkSandbox(sandboxName string) CheckResult {
	if sandboxName == "" {
		return CheckResult{
			Name:    "Sandbox",
			Status:  "WARN",
			Message: "Cannot determine sandbox name",
		}
	}

	sb, err := sandbox.Get(sandboxName)
	if err != nil {
		return CheckResult{
			Name:    "Sandbox",
			Status:  "WARN",
			Message: "Failed to check sandbox status",
		}
	}

	if sb == nil {
		return CheckResult{
			Name:    "Sandbox",
			Status:  "WARN",
			Message: "Not found",
			Hint:    "Will be created on first run",
		}
	}

	return CheckResult{
		Name:    "Sandbox",
		Status:  "OK",
		Message: fmt.Sprintf("%s (%s)", sb.Name, sb.Status),
	}
}

func printGreen(text string) {
	fmt.Printf("\033[32m%s\033[0m\n", text)
}

func printYellow(text string) {
	fmt.Printf("\033[33m%s\033[0m\n", text)
}

func printRed(text string) {
	fmt.Printf("\033[31m%s\033[0m\n", text)
}