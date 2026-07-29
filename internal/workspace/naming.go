// Package workspace provides workspace management functionality for cloma.
// It handles path resolution, naming conventions, and random workspace creation.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PathToSlug converts a path basename to a slug format.
// It lowercases the basename and replaces non-alphanumeric characters with hyphens.
// Consecutive hyphens are collapsed, and leading/trailing hyphens are removed.
func PathToSlug(path string) string {
	return slugify(filepath.Base(path))
}

// slugify converts an arbitrary string into a lowercase, hyphen-delimited slug
// suitable for use in a sandbox name. Non-alphanumeric characters become
// hyphens, consecutive hyphens are collapsed, and leading/trailing hyphens
// are removed.
func slugify(s string) string {
	// Convert to lowercase
	slug := strings.ToLower(s)

	// Replace non-alphanumeric characters with hyphens
	re := regexp.MustCompile(`[^a-z0-9]`)
	slug = re.ReplaceAllString(slug, "-")

	// Collapse consecutive hyphens
	re2 := regexp.MustCompile(`-+`)
	slug = re2.ReplaceAllString(slug, "-")

	// Remove leading hyphen
	slug = strings.TrimPrefix(slug, "-")

	// Remove trailing hyphen
	slug = strings.TrimSuffix(slug, "-")

	return slug
}

// PathHash generates an 8-character SHA256 hash of the given path.
// This provides uniqueness for sandbox names when combined with the slug.
func PathHash(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])[:8]
}

// Docker Desktop creates a VM ethernet socket at:
//
//	~/.docker/sandboxes/vm/<sandboxName>/eth
//
// The total path length must not exceed vmSocketPathMaxLen characters (the
// limit enforced by the LinuxKit VM networking layer). The prefix portion
// (home + "/.docker/sandboxes/vm/") and the "/eth" suffix are fixed, so the
// remaining budget is consumed by the sandbox name. We compute the limit
// dynamically from the user's home directory, falling back to a conservative
// constant when the home directory cannot be determined.
const (
	vmSocketPathSuffix  = "/eth"
	vmSocketPathSubPath = "/.docker/sandboxes/vm/"
	vmSocketPathMaxLen  = 94
)

// maxSandboxNameLength returns the maximum number of characters available for
// the sandbox name given the user's home directory and the fixed VM socket
// path components.
func maxSandboxNameLength() int {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Conservative fallback: assume a 24-char home directory.
		home = "/home/user"
	}
	return maxSandboxNameLengthForHome(home)
}

// maxSandboxNameLengthForHome computes the sandbox name length budget for a
// given home directory. This is split out so tests can exercise the truncation
// logic with specific home directory lengths.
func maxSandboxNameLengthForHome(home string) int {
	// fixedLen = len(home) + len(vmSocketPathSubPath) + len(sandboxName) + len(vmSocketPathSuffix)
	// Solve for sandboxName: sandboxName <= vmSocketPathMaxLen - len(home) - len(vmSocketPathSubPath) - len(vmSocketPathSuffix)
	fixedLen := len(home) + len(vmSocketPathSubPath) + len(vmSocketPathSuffix)
	max := vmSocketPathMaxLen - fixedLen
	// The minimum viable name is "cloma-" + 8-char hash = 14 chars.
	if max < 14 {
		return 14
	}
	return max
}

// SandboxName generates a sandbox name from a workspace path.
// The format is: cloma-{slug}-{hash}
// Example: cloma-myproject-a1b2c3d4
//
// The slug is truncated when necessary so that the full sandbox name fits
// within the length budget imposed by the Docker Desktop VM ethernet socket
// path limit. The 8-character hash always remains intact, guaranteeing
// uniqueness.
func SandboxName(workspace string) string {
	return sandboxNameWithMaxLen(workspace, maxSandboxNameLength())
}

// sandboxNameWithMaxLen builds the sandbox name within the given character
// budget. It is split out so tests can exercise the truncation logic with
// specific budgets.
func sandboxNameWithMaxLen(workspace string, maxName int) string {
	slug := PathToSlug(workspace)
	hash := PathHash(workspace)

	const prefix = "cloma-"

	// Minimum name without a slug: "cloma-" + hash = 14 chars.
	if maxName <= len(prefix)+len(hash) {
		// Extremely tight budget: drop the slug entirely and truncate if
		// needed. This should never happen in practice.
		name := prefix + hash
		if len(name) > maxName {
			name = name[:maxName]
		}
		return name
	}

	// With a slug: "cloma-" + slug + "-" + hash.
	reserved := len(prefix) + 1 + len(hash) // prefix + hyphen + hash

	if reserved+len(slug) <= maxName {
		// Full slug fits.
		return prefix + slug + "-" + hash
	}

	// Truncate the slug to fit. Leave room for a trailing hyphen before the
	// hash so the name stays well-formed.
	slugBudget := maxName - reserved
	if slugBudget < 1 {
		// No room for any slug characters; use hash-only name.
		return prefix + hash
	}
	slug = slug[:slugBudget]
	// Trim a trailing hyphen left by truncation for cleanliness.
	slug = strings.TrimSuffix(slug, "-")
	if slug == "" {
		return prefix + hash
	}
	return prefix + slug + "-" + hash
}

// ErrInvalidSandboxLabel is returned when a user-supplied sandbox label does
// not produce a usable sandbox name (e.g. it contains no alphanumeric
// characters after slugification).
var ErrInvalidSandboxLabel = errors.New("sandbox name must contain at least one alphanumeric character")

// ResolveSandboxName builds a cloma-managed sandbox name from a user-supplied
// label (the value passed to --name). The label is slugified; if it does not
// already start with the "cloma-" prefix, the prefix is added so the sandbox
// is recognized by `cloma list`. Passing an already-prefixed name (e.g. one
// copied from `cloma list`) is idempotent.
//
// Unlike SandboxName, no path hash is appended: the caller owns the
// uniqueness of the label, which is what lets several instances share the
// same workspace directory without colliding.
//
// The name is truncated when necessary so it fits within the length budget
// imposed by the Docker Desktop VM ethernet socket path limit.
func ResolveSandboxName(label string) (string, error) {
	return resolveSandboxNameWithMaxLen(label, maxSandboxNameLength())
}

// resolveSandboxNameWithMaxLen is split out so tests can exercise the
// truncation logic with a specific character budget.
func resolveSandboxNameWithMaxLen(label string, maxName int) (string, error) {
	slug := slugify(label)
	if slug == "" {
		return "", ErrInvalidSandboxLabel
	}

	const prefix = "cloma-"
	if !strings.HasPrefix(slug, prefix) {
		slug = prefix + slug
	}

	if len(slug) <= maxName {
		return slug, nil
	}

	// Truncate to fit the budget, trimming a trailing hyphen for cleanliness.
	slug = strings.TrimSuffix(slug[:maxName], "-")
	if slug == "" {
		return "", ErrInvalidSandboxLabel
	}
	return slug, nil
}
