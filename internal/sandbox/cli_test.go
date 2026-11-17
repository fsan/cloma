package sandbox

import (
	"os"
	"path/filepath"
	"reflect"
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
