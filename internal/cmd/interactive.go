package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fsan/cloma/internal/config"
	"github.com/fsan/cloma/internal/ollama"
	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
)

// agentChoices lists the selectable code agents with their display labels,
// in the order they appear in the interactive picker.
var agentChoices = []struct {
	value string
	label string
}{
	{sandbox.AgentClaude, "Claude Code"},
	{sandbox.AgentGrok, "Grok Build"},
	{sandbox.AgentKimi, "Kimi Code"},
	{sandbox.AgentOpenClaw, "OpenClaw"},
	{sandbox.AgentJunie, "Junie CLI"},
	{sandbox.AgentPi, "Pi coding agent"},
}

// interactiveStdin is the buffered reader shared by all prompts so answers
// typed ahead are not lost between prompts.
var interactiveStdin *bufio.Reader

// errInteractiveAborted is returned when the user declines the final
// confirmation (or closes stdin) during the interactive setup.
var errInteractiveAborted = errors.New("interactive setup aborted")

// readAnswer reads one trimmed line from stdin. It accepts a final line
// without a trailing newline; an exhausted stdin (Ctrl-D) is reported as
// errInteractiveAborted so the wizard stops with a clear message instead
// of spinning on empty input or surfacing a raw EOF.
func readAnswer() (string, error) {
	if interactiveStdin == nil {
		interactiveStdin = bufio.NewReader(os.Stdin)
	}
	line, err := interactiveStdin.ReadString('\n')
	if err != nil && line == "" {
		return "", errInteractiveAborted
	}
	return strings.TrimSpace(line), nil
}

// promptString asks a free-text question and returns the answer, falling
// back to def when the user just presses Enter.
func promptString(label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	answer, err := readAnswer()
	if err != nil {
		return "", err
	}
	if answer == "" {
		return def, nil
	}
	return answer, nil
}

// promptSelect shows a numbered list of options and returns the chosen index.
// Pressing Enter picks the default; invalid numbers re-prompt.
func promptSelect(label string, options []string, defIdx int) (int, error) {
	fmt.Printf("%s:\n", label)
	for i, opt := range options {
		def := ""
		if i == defIdx {
			def = " (default)"
		}
		fmt.Printf("  %2d) %s%s\n", i+1, opt, def)
	}
	for {
		fmt.Printf("Select [1-%d, default %d]: ", len(options), defIdx+1)
		answer, err := readAnswer()
		if err != nil {
			return 0, err
		}
		if answer == "" {
			return defIdx, nil
		}
		n, err := strconv.Atoi(answer)
		if err != nil || n < 1 || n > len(options) {
			fmt.Printf("  Enter a number between 1 and %d, or press Enter for the default.\n", len(options))
			continue
		}
		return n - 1, nil
	}
}

// promptPort asks for a TCP port number, falling back to def on Enter.
func promptPort(label string, def int) (int, error) {
	for {
		fmt.Printf("%s [%d]: ", label, def)
		answer, err := readAnswer()
		if err != nil {
			return 0, err
		}
		if answer == "" {
			return def, nil
		}
		n, err := strconv.Atoi(answer)
		if err != nil || n < 1 || n > 65535 {
			fmt.Println("  Enter a valid port number (1-65535).")
			continue
		}
		return n, nil
	}
}

// promptBool asks a yes/no question, falling back to def on Enter.
func promptBool(label string, def bool) (bool, error) {
	defStr := "y/N"
	if def {
		defStr = "Y/n"
	}
	for {
		fmt.Printf("%s [%s]: ", label, defStr)
		answer, err := readAnswer()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("  Please answer y or n.")
		}
	}
}

// interactiveSetup fills in the run options one prompt at a time. The
// required parameters (workspace, Ollama port, model, agent) are asked
// first, then the optional ones (instance name, extra agent flags, env
// vars, tmpfs workspace). Values already passed as flags (e.g.
// `cloma -m <model> -i`) or set in the config act as prompt defaults, so
// `-i` can be combined with flags to skip the prompts you care about.
// A final confirmation shows the resolved configuration before anything
// is created.
func interactiveSetup() error {
	fmt.Println("=== Cloma interactive setup ===")
	fmt.Println("Press Enter to accept the [default]. Ctrl-C aborts.")
	fmt.Println()

	// --- Required parameters ---

	// Workspace: must resolve to an existing directory (or one we offer to
	// create). Defaults to the current directory.
	def := runWorkspace
	if def == "" {
		def, _ = os.Getwd()
	}
	for {
		ws, err := promptString("Workspace directory", def)
		if err != nil {
			return err
		}
		resolved, rerr := workspace.Resolve(ws)
		if rerr == nil {
			runWorkspace = resolved
			break
		}
		create, err := promptBool(fmt.Sprintf("%q does not exist. Create it", ws), true)
		if err != nil {
			return err
		}
		if !create {
			def = ws
			continue
		}
		abs, aerr := expandHome(ws)
		if aerr != nil {
			return aerr
		}
		if err := os.MkdirAll(abs, 0755); err != nil {
			return fmt.Errorf("failed to create workspace %s: %w", abs, err)
		}
		runWorkspace = abs
		break
	}

	// Ollama port: asked before the model so the model picker can list what
	// is actually running on that port.
	defPort := runPort
	if defPort == 0 {
		defPort = config.GetOllamaPort()
	}
	port, err := promptPort("Ollama port", defPort)
	if err != nil {
		return err
	}
	runPort = port

	// Model: pick from the models Ollama reports on the chosen port when it
	// is reachable; otherwise fall back to free-text entry.
	client := ollama.NewClient(fmt.Sprintf("http://localhost:%d", port))
	defModel := runModel
	if defModel == "" {
		defModel = config.GetModel()
	}
	models, merr := client.GetModels()
	if merr != nil {
		fmt.Printf("Ollama not reachable on port %d; enter the model name manually.\n", port)
		runModel, err = promptString("Model", defModel)
		if err != nil {
			return err
		}
	} else if len(models) == 0 {
		fmt.Printf("Ollama has no models pulled on port %d; enter the model name manually.\n", port)
		runModel, err = promptString("Model", defModel)
		if err != nil {
			return err
		}
	} else {
		defIdx := 0
		for i, m := range models {
			if m == defModel {
				defIdx = i
				break
			}
		}
		options := append(append([]string{}, models...), "other (enter a model name)")
		idx, err := promptSelect("Model", options, defIdx)
		if err != nil {
			return err
		}
		if idx < len(models) {
			runModel = models[idx]
		} else {
			for {
				custom, err := promptString("Model name", defModel)
				if err != nil {
					return err
				}
				if !client.ModelExists(custom) {
					fmt.Printf("  Model %q not found in Ollama; pull it first or re-enter.\n", custom)
					continue
				}
				runModel = custom
				break
			}
		}
	}

	// Agent: numbered picker, defaulting to the configured agent.
	defAgent := sandbox.NormalizeAgent(runAgent)
	if runAgent == "" {
		defAgent = sandbox.NormalizeAgent(config.GetAgent())
	}
	labels := make([]string, len(agentChoices))
	defIdx := 0
	for i, a := range agentChoices {
		labels[i] = a.label
		if a.value == defAgent {
			defIdx = i
		}
	}
	idx, err := promptSelect("Code agent", labels, defIdx)
	if err != nil {
		return err
	}
	runAgent = agentChoices[idx].value

	// --- Optional parameters ---

	fmt.Println()
	fmt.Println("Optional settings (press Enter to accept the default):")

	runName, err = promptString("Instance name (empty = auto-derived from workspace)", runName)
	if err != nil {
		return err
	}

	runFlags, err = promptString("Extra agent flags (empty = none)", runFlags)
	if err != nil {
		return err
	}

	fmt.Println("Environment variables to inject into the sandbox (KEY=VALUE).")
	for {
		e, err := promptString("Add env var (empty = done)", "")
		if err != nil {
			return err
		}
		if e == "" {
			break
		}
		if !isValidEnvAssignment(e) {
			fmt.Println("  Invalid value: expected KEY=VALUE form (e.g. DEBUG=1)")
			continue
		}
		runEnv = append(runEnv, e)
	}

	useTempfs, err := promptBool("Use an ephemeral in-memory (tmpfs) workspace instead?", runTempfs)
	if err != nil {
		return err
	}
	runTempfs = useTempfs
	if useTempfs {
		for {
			size, err := promptString("Tmpfs size", runTempfsSize)
			if err != nil {
				return err
			}
			if !isValidTmpfsSize(size) {
				fmt.Println("  Invalid size: expected a number with an optional k/m/g unit (e.g. 512m, 1g)")
				continue
			}
			runTempfsSize = size
			break
		}
	}

	// --- Confirmation ---

	agent := sandbox.NormalizeAgent(runAgent)
	fmt.Println()
	fmt.Println("=== Configuration ===")
	fmt.Printf("  Agent:     %s (%s)\n", agentChoices[idx].label, agent)
	fmt.Printf("  Model:     %s\n", runModel)
	fmt.Printf("  Ollama:    http://localhost:%d\n", runPort)
	if runTempfs {
		fmt.Printf("  Workspace: (tmpfs, %s)\n", runTempfsSize)
	} else {
		fmt.Printf("  Workspace: %s\n", runWorkspace)
	}
	if runName != "" {
		fmt.Printf("  Name:      %s\n", runName)
	}
	if runFlags != "" {
		fmt.Printf("  Flags:     %s\n", runFlags)
	}
	if len(runEnv) > 0 {
		fmt.Printf("  Env:       %s\n", strings.Join(runEnv, " "))
	}
	fmt.Println()

	proceed, err := promptBool("Proceed", true)
	if err != nil {
		return err
	}
	if !proceed {
		return errInteractiveAborted
	}
	return nil
}

// isValidTmpfsSize reports whether size is a valid tmpfs size option: a
// number with an optional k/K, m/M or g/G unit (e.g. "512m", "1g", "1024"),
// or empty for the kernel default. The value is passed verbatim to
// `mount -o size=<size>`, so anything else would fail at mount time.
func isValidTmpfsSize(size string) bool {
	if size == "" {
		return true
	}
	unit := ""
	last := size[len(size)-1]
	if !strings.ContainsAny(string(last), "0123456789") {
		unit = string(last)
		size = size[:len(size)-1]
	}
	if size == "" || strings.Trim(size, "0123456789") != "" {
		return false
	}
	switch unit {
	case "", "k", "K", "m", "M", "g", "G":
		return true
	}
	return false
}

// expandHome expands a leading ~ or ~/ in the given path and makes it
// absolute. Used by the wizard when creating a workspace directory that does
// not exist yet (workspace.Resolve only handles existing paths).
func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}
