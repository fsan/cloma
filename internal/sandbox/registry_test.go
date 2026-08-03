package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempHome sets the HOME environment variable to a temp directory for the
// duration of a test so registry functions read/write to an isolated location.
// It returns the temp home path and a cleanup function.
func withTempHome(t *testing.T) (string, func()) {
	t.Helper()
	origHome, hadHome := os.LookupEnv("HOME")

	dir, err := os.MkdirTemp("", "cloma-registry-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	os.Setenv("HOME", dir)

	cleanup := func() {
		if hadHome {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		os.RemoveAll(dir)
	}
	return dir, cleanup
}

func TestFindByWorkspace(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	wsA := "/Users/test/project-a"
	wsB := "/Users/test/project-b"

	// Store two sandbox entries: one label-based, one path-based.
	if err := StoreWorkspace("cloma-instance1", wsA); err != nil {
		t.Fatalf("StoreWorkspace: %v", err)
	}
	if err := StoreWorkspace("cloma-myproject-a1b2c3d4", wsB); err != nil {
		t.Fatalf("StoreWorkspace: %v", err)
	}

	// Exact match for workspace A → label-based sandbox.
	got, err := FindByWorkspace(wsA)
	if err != nil {
		t.Fatalf("FindByWorkspace(%q) error: %v", wsA, err)
	}
	if got != "cloma-instance1" {
		t.Errorf("FindByWorkspace(%q) = %q, want %q", wsA, got, "cloma-instance1")
	}

	// Exact match for workspace B → path-based sandbox.
	got, err = FindByWorkspace(wsB)
	if err != nil {
		t.Fatalf("FindByWorkspace(%q) error: %v", wsB, err)
	}
	if got != "cloma-myproject-a1b2c3d4" {
		t.Errorf("FindByWorkspace(%q) = %q, want %q", wsB, got, "cloma-myproject-a1b2c3d4")
	}

	// No match for unknown workspace.
	got, err = FindByWorkspace("/nonexistent")
	if err != nil {
		t.Fatalf("FindByWorkspace(/nonexistent) error: %v", err)
	}
	if got != "" {
		t.Errorf("FindByWorkspace(/nonexistent) = %q, want empty", got)
	}
}

func TestFindByWorkspaceEmptyRegistry(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	// No entries at all — should return "" with no error.
	got, err := FindByWorkspace("/some/path")
	if err != nil {
		t.Fatalf("FindByWorkspace error on empty registry: %v", err)
	}
	if got != "" {
		t.Errorf("FindByWorkspace on empty registry = %q, want empty", got)
	}
}

func TestFindByWorkspaceIgnoresNonClomaEntries(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	// Manually create a non-cloma entry in the registry dir.
	regDir := filepath.Join(home, ".cloma", sandboxRegistryDir)
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "other-sandbox"), []byte("/some/path"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Should not match non-cloma entries.
	got, err := FindByWorkspace("/some/path")
	if err != nil {
		t.Fatalf("FindByWorkspace error: %v", err)
	}
	if got != "" {
		t.Errorf("FindByWorkspace matched non-cloma entry: %q", got)
	}
}
