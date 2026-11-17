package sandbox

import (
	"os/exec"
)

// Backend identifies which sandbox command-line interface cloma drives.
//
// Docker Desktop used to ship the `docker sandbox` plugin. Newer releases
// removed it in favor of the standalone `sbx` CLI, so cloma prefers `sbx` when
// it is installed and falls back to the legacy plugin for older installs.
type Backend struct {
	// Binary is the executable to run: "sbx" or "docker".
	Binary string

	// Prefix is the subcommand path prepended to every invocation. It is
	// empty for sbx ("sbx ls") and {"sandbox"} for the legacy plugin
	// ("docker sandbox ls").
	Prefix []string
}

// DetectBackend returns the sandbox backend available on PATH. The standalone
// sbx CLI wins when both are installed.
func DetectBackend() Backend {
	if _, err := exec.LookPath("sbx"); err == nil {
		return Backend{Binary: "sbx"}
	}
	return Backend{Binary: "docker", Prefix: []string{"sandbox"}}
}

// IsSBX reports whether the backend is the standalone sbx CLI rather than the
// legacy `docker sandbox` plugin.
func (b Backend) IsSBX() bool {
	return len(b.Prefix) == 0
}

// Args returns the full argument list for a subcommand invocation.
func (b Backend) Args(args ...string) []string {
	out := make([]string, 0, len(b.Prefix)+len(args))
	out = append(out, b.Prefix...)
	return append(out, args...)
}

// Command builds an *exec.Cmd for the given subcommand arguments.
func (b Backend) Command(args ...string) *exec.Cmd {
	return exec.Command(b.Binary, b.Args(args...)...)
}

// Command builds an *exec.Cmd against the auto-detected backend.
func Command(args ...string) *exec.Cmd {
	return DetectBackend().Command(args...)
}
