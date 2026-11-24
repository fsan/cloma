package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeFakeCLI drops an executable stub named name into dir so exec.LookPath
// finds it.
func writeFakeCLI(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write fake %s: %v", name, err)
	}
}

func TestDetectBackendPrefersSBX(t *testing.T) {
	dir := t.TempDir()
	writeFakeCLI(t, dir, "sbx")
	writeFakeCLI(t, dir, "docker")
	t.Setenv("PATH", dir)

	backend := DetectBackend()
	if !backend.IsSBX() {
		t.Fatalf("DetectBackend() with sbx on PATH = %+v, want the sbx backend", backend)
	}
	if backend.Binary != "sbx" {
		t.Errorf("DetectBackend().Binary = %q, want %q", backend.Binary, "sbx")
	}

	want := []string{"exec", "-u", "agent", "my-sandbox", "bash"}
	if got := backend.Args("exec", "-u", "agent", "my-sandbox", "bash"); !reflect.DeepEqual(got, want) {
		t.Errorf("sbx Args() = %v, want %v", got, want)
	}
}

func TestDetectBackendFallsBackToDocker(t *testing.T) {
	dir := t.TempDir()
	writeFakeCLI(t, dir, "docker")
	t.Setenv("PATH", dir)

	backend := DetectBackend()
	if backend.IsSBX() {
		t.Fatalf("DetectBackend() without sbx = %+v, want the docker sandbox backend", backend)
	}
	if backend.Binary != "docker" {
		t.Errorf("DetectBackend().Binary = %q, want %q", backend.Binary, "docker")
	}

	want := []string{"sandbox", "ls", "--json"}
	if got := backend.Args("ls", "--json"); !reflect.DeepEqual(got, want) {
		t.Errorf("docker Args() = %v, want %v", got, want)
	}
}

func TestCommandError(t *testing.T) {
	base := errors.New("exit status 1")

	err := CommandError("failed to list sandboxes", base, []byte("error: not signed in to Docker\n  try: sbx login\n"))
	if !strings.Contains(err.Error(), "not signed in to Docker") || !strings.Contains(err.Error(), "login") {
		t.Errorf("signed-out error should name the cause and the fix, got %q", err)
	}

	err = CommandError("failed to stop sandbox x", base, []byte("boom"))
	if !errors.Is(err, base) || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should wrap the cause and include the output, got %q", err)
	}

	err = CommandError("failed", base, nil)
	if !errors.Is(err, base) {
		t.Errorf("empty output should still wrap the cause, got %q", err)
	}
}
