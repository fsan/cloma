package workspace

import (
	"os"
	"strings"
	"testing"
)

func TestPathToSlug(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/home/user/myproject",
			expected: "myproject",
		},
		{
			name:     "path with spaces",
			path:     "/home/user/My Project",
			expected: "my-project",
		},
		{
			name:     "path with special chars",
			path:     "/home/user/my-project_2024",
			expected: "my-project-2024",
		},
		{
			name:     "path with mixed case",
			path:     "/home/user/MyProject",
			expected: "myproject",
		},
		{
			name:     "path with trailing special char",
			path:     "/home/user/project!",
			expected: "project",
		},
		{
			name:     "path with leading special char",
			path:     "/home/user/.hidden",
			expected: "hidden",
		},
		{
			name:     "path with multiple special chars",
			path:     "/home/user/My___Project!!!",
			expected: "my-project",
		},
		{
			name:     "current directory",
			path:     ".",
			expected: "", // "." becomes "-", which gets stripped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PathToSlug(tt.path)
			if result != tt.expected {
				t.Errorf("PathToSlug(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestPathHash(t *testing.T) {
	// Test that hashes are consistent
	path1 := "/home/user/project"
	hash1 := PathHash(path1)
	hash2 := PathHash(path1)

	if hash1 != hash2 {
		t.Errorf("PathHash not consistent: %q != %q", hash1, hash2)
	}

	// Test that hashes are 8 characters
	if len(hash1) != 8 {
		t.Errorf("PathHash length = %d, want 8", len(hash1))
	}

	// Test that different paths produce different hashes
	path2 := "/home/user/other"
	hash3 := PathHash(path2)
	if hash1 == hash3 {
		t.Errorf("PathHash collision: %q and %q both produce %q", path1, path2, hash1)
	}
}

func TestSandboxName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		contains string
	}{
		{
			name:     "simple path",
			path:     "/home/user/myproject",
			contains: "cloma-myproject-",
		},
		{
			name:     "path with spaces",
			path:     "/home/user/My Project",
			contains: "cloma-my-project-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SandboxName(tt.path)
			// Verify length is adequate (cloma- + slug + - + 8-char hash)
			if len(result) < 15 {
				t.Errorf("SandboxName(%q) = %q, too short", tt.path, result)
			}
			// Check prefix
			prefix := "cloma-"
			if result[:len(prefix)] != prefix {
				t.Errorf("SandboxName(%q) = %q, should start with %q", tt.path, result, prefix)
			}
			// Verify it contains the expected substring
			if len(result) >= len(tt.contains) && result[:len(tt.contains)] != tt.contains {
				t.Errorf("SandboxName(%q) = %q, should start with %q", tt.path, result, tt.contains)
			}
		})
	}
}

// TestSandboxNameSocketPathLimit verifies that the generated sandbox name,
// when placed in the Docker Desktop VM ethernet socket path, never exceeds
// the 94-character limit enforced by the LinuxKit VM networking layer.
func TestSandboxNameSocketPathLimit(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	// Simulate the real VM socket path: ~/.docker/sandboxes/vm/<name>/eth
	tests := []struct {
		name string
		path string
	}{
		{"short name", "/home/user/myproject"},
		{"long name", "/Users/fabio-sancinetticookpad.com/workspace/personal/gloma/global-search-quality"},
		{"very long name", "/Users/fabio-sancinetticookpad.com/workspace/personal/gloma/some-very-long-workspace-name-that-could-cause-issues"},
		{"path with spaces and special chars", "/Users/fabio-sancinetticookpad.com/My Project!!! ___ test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandboxName := SandboxName(tt.path)

			// Construct the full socket path the way Docker Desktop does.
			socketPath := home + "/.docker/sandboxes/vm/" + sandboxName + "/eth"

			if len(socketPath) > vmSocketPathMaxLen {
				t.Errorf("socket path too long: %d > %d\n  path: %s\n  sandbox: %s",
					len(socketPath), vmSocketPathMaxLen, socketPath, sandboxName)
			}

			// The name must always start with "cloma-" and contain the hash.
			if len(sandboxName) < len("cloma-")+8 {
				t.Errorf("sandbox name too short: %q (%d chars)", sandboxName, len(sandboxName))
			}
		})
	}
}

// TestSandboxNameTruncationPreservesHash verifies that the 8-character hash is
// always present at the end of the sandbox name, even when the slug is
// truncated.
func TestSandboxNameTruncationPreservesHash(t *testing.T) {
	path := "/Users/fabio-sancinetticookpad.com/workspace/personal/gloma/global-search-quality"
	hash := PathHash(path)

	sandboxName := SandboxName(path)

	// The hash must be the suffix of the sandbox name.
	if !strings.HasSuffix(sandboxName, hash) {
		t.Errorf("sandbox name %q does not end with hash %q", sandboxName, hash)
	}
}

// TestSandboxNameTruncationWithLongHomeDir simulates the exact failing scenario
// from the bug report: a long home directory like
// /Users/fabio-sancinetticookpad.com (34 chars) combined with a long workspace
// slug would exceed the 94-char socket path limit.
func TestSandboxNameTruncationWithLongHomeDir(t *testing.T) {
	// Simulate the exact scenario from the bug report.
	home := "/Users/fabio-sancinetticookpad.com" // 34 chars
	maxName := maxSandboxNameLengthForHome(home)

	// expected: 94 - 34 - 22 - 4 = 34
	//   home=34, "/.docker/sandboxes/vm/"=22, "/eth"=4
	if maxName != 34 {
		t.Fatalf("maxSandboxNameLengthForHome(%q) = %d, want 34", home, maxName)
	}

	path := "/Users/fabio-sancinetticookpad.com/workspace/personal/gloma/global-search-quality"
	hash := PathHash(path)
	name := sandboxNameWithMaxLen(path, maxName)

	// The full name must fit within the budget.
	if len(name) > maxName {
		t.Errorf("name %q is %d chars, exceeds budget of %d", name, len(name), maxName)
	}

	// The hash must be preserved at the end.
	if !strings.HasSuffix(name, hash) {
		t.Errorf("name %q does not end with hash %q", name, hash)
	}

	// Verify the full socket path fits.
	socketPath := home + "/.docker/sandboxes/vm/" + name + "/eth"
	if len(socketPath) > vmSocketPathMaxLen {
		t.Errorf("socket path too long: %d > %d\n  %s", len(socketPath), vmSocketPathMaxLen, socketPath)
	}

	// The name should still start with "cloma-".
	if !strings.HasPrefix(name, "cloma-") {
		t.Errorf("name %q does not start with cloma-", name)
	}
}

// TestSandboxNameNoTruncationWhenShort verifies that when the budget is
// generous, the full slug is preserved without truncation.
func TestSandboxNameNoTruncationWhenShort(t *testing.T) {
	path := "/home/user/myproject"
	hash := PathHash(path)
	name := sandboxNameWithMaxLen(path, 200) // generous budget

	expected := "cloma-myproject-" + hash
	if name != expected {
		t.Errorf("got %q, want %q", name, expected)
	}
}

// TestSandboxNameHashOnlyWhenBudgetTiny verifies that with an extremely tight
// budget, the name degrades to "cloma-" + hash.
func TestSandboxNameHashOnlyWhenBudgetTiny(t *testing.T) {
	path := "/home/user/myproject"
	hash := PathHash(path)
	name := sandboxNameWithMaxLen(path, 14) // exactly "cloma-" + 8-char hash

	expected := "cloma-" + hash
	if name != expected {
		t.Errorf("got %q, want %q", name, expected)
	}
}
