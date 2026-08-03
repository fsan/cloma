package cmd

import (
	"fmt"

	"github.com/fsan/cloma/internal/sandbox"
	"github.com/fsan/cloma/internal/workspace"
)

// resolveSandboxNameFromWorkspace derives the sandbox name from a workspace
// path, with a fallback to the sandbox registry. It first computes the
// path-derived name (cloma-{slug}-{hash}). If that name has no registry entry
// — which happens when the sandbox was created with --name (a label-based
// name without a path hash) — it searches the registry for a sandbox whose
// stored workspace matches the resolved path.
//
// Returns the sandbox name, the resolved workspace path, and any error.
func resolveSandboxNameFromWorkspace(workspacePath string) (string, string, error) {
	if workspacePath == "" {
		workspacePath = "."
	}

	resolvedWorkspace, err := workspace.Resolve(workspacePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve workspace: %w\nHint: Ensure the path exists: %s", err, workspacePath)
	}

	sandboxName := workspace.SandboxName(resolvedWorkspace)

	// If the path-derived name has no registry entry, search for a sandbox
	// created with --name from this workspace.
	if stored, _ := sandbox.GetStoredWorkspace(sandboxName); stored == "" {
		if found, _ := sandbox.FindByWorkspace(resolvedWorkspace); found != "" {
			sandboxName = found
		}
	}

	return sandboxName, resolvedWorkspace, nil
}
