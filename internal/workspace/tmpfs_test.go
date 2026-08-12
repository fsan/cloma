package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempHome sets HOME to an isolated temp directory for the test and
// restores it on cleanup. TmpfsBaseDir derives from HOME, so this keeps
// IsTmpfsWorkspace tests from touching the real state directory.
func withTempHome(t *testing.T) (string, func()) {
	t.Helper()
	origHome, hadHome := os.LookupEnv("HOME")
	dir, err := os.MkdirTemp("", "cloma-tmpfs-test-*")
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

func TestIsTmpfsWorkspace(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	base := TmpfsBaseDir()

	// Redirect the /tmp fallback base to an isolated temp dir so the test does
	// not touch the real /tmp.
	tmpBase, err := os.MkdirTemp("", "cloma-tmpfs-fallback-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpBase)
	origTempBase := tempDirBase
	tempDirBase = tmpBase
	defer func() { tempDirBase = origTempBase }()

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"under tmpfs base", filepath.Join(base, "cloma-foo-a1b2c3d4"), true},
		{"tmpfs base itself", base, false}, // rel == ".", not a child
		{"parent escape", filepath.Join(base, "..", "etc"), false},
		{"outside both bases", "/home/user/projects/foo", false},
		{"fallback dir under temp base", filepath.Join(tmpBase, "cloma-foo"), true},
		{"non-cloma dir under temp base", filepath.Join(tmpBase, "other"), false},
		{"nested under temp base (not a direct child)", filepath.Join(tmpBase, "cloma-foo", "sub"), false},
		{"temp base itself", tmpBase, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTmpfsWorkspace(c.path); got != c.want {
				t.Errorf("IsTmpfsWorkspace(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestTmpfsBaseDirUnderClomaState(t *testing.T) {
	home, cleanup := withTempHome(t)
	defer cleanup()

	base := TmpfsBaseDir()
	want := filepath.Join(home, ".cloma", TmpfsSubDir)
	if base != want {
		t.Errorf("TmpfsBaseDir() = %q, want %q", base, want)
	}
}

func TestRandomSandboxName(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		name, err := RandomSandboxName()
		if err != nil {
			t.Fatalf("RandomSandboxName: %v", err)
		}
		const prefix = "cloma-"
		if name[:len(prefix)] != prefix {
			t.Errorf("RandomSandboxName() = %q, want %q prefix", name, prefix)
		}
		suffix := name[len(prefix):]
		if len(suffix) != 8 {
			t.Errorf("RandomSandboxName() suffix length = %d, want 8", len(suffix))
		}
		if seen[name] {
			t.Errorf("RandomSandboxName() returned duplicate %q", name)
		}
		seen[name] = true
	}
}

// TestCreateTmpfsFallback exercises the plain-directory fallback path used on
// macOS (or unprivileged Linux) by disabling the real mount attempt and
// redirecting the fallback base to an isolated temp directory.
func TestCreateTmpfsFallback(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	tmpBase, err := os.MkdirTemp("", "cloma-tmpfs-create-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpBase)

	origTempBase := tempDirBase
	origAttempt := attemptTmpfsMount
	tempDirBase = tmpBase
	attemptTmpfsMount = false
	defer func() {
		tempDirBase = origTempBase
		attemptTmpfsMount = origAttempt
	}()

	name := "cloma-fallback-test"
	path, mounted, err := CreateTmpfs(name, "1g")
	if err != nil {
		t.Fatalf("CreateTmpfs: %v", err)
	}
	if mounted {
		t.Errorf("mounted = true, want false for fallback")
	}
	want := filepath.Join(tmpBase, name)
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if !IsTmpfsWorkspace(path) {
		t.Errorf("IsTmpfsWorkspace(%q) = false, want true", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("fallback dir not created: %v", err)
	}

	if err := UnmountTmpfs(path); err != nil {
		t.Fatalf("UnmountTmpfs: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("fallback dir still exists after UnmountTmpfs")
	}
}
