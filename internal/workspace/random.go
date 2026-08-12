package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// DefaultWorkspacesDir is the default directory for random workspaces.
const DefaultWorkspacesDir = "~/.cloma/workspaces"

// ErrCreateWorkspace is returned when a random workspace cannot be created.
var ErrCreateWorkspace = errors.New("failed to create random workspace")

// CreateRandom creates a random workspace directory in ~/.cloma/workspaces/.
// The directory name follows the pattern: cloma-XXXXXX (where X is random hex).
// It returns the absolute path of the created directory.
func CreateRandom() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	baseDir := filepath.Join(home, ".cloma", "workspaces")

	// Ensure the base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	// Try to create a unique directory
	const maxAttempts = 100
	for i := 0; i < maxAttempts; i++ {
		randomSuffix, err := randomHex(6)
		if err != nil {
			return "", err
		}

		dirPath := filepath.Join(baseDir, "cloma-"+randomSuffix)

		// Try to create the directory (MkdirAll will fail if it exists)
		err = os.Mkdir(dirPath, 0755)
		if err == nil {
			return dirPath, nil
		}

		// If directory already exists, try again with a new random suffix
		if os.IsExist(err) {
			continue
		}

		// Other errors are fatal
		return "", err
	}

	return "", ErrCreateWorkspace
}

// randomHex generates n bytes of random data and returns it as a hex string.
func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// RandomSandboxName returns a random cloma-prefixed sandbox name with an
// 8-character hex suffix. It is used when there is no real workspace path to
// derive the name from, e.g. when --tempfs is used without --name.
func RandomSandboxName() (string, error) {
	suffix, err := randomHex(4) // 4 bytes -> 8 hex chars
	if err != nil {
		return "", err
	}
	return "cloma-" + suffix, nil
}
