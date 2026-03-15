// Package config manages configuration for the cloma CLI.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Default configuration values.
const (
	// DefaultModel is the default AI model to use.
	DefaultModel = "glm-5:cloud"

	// DefaultOllamaPort is the default port for Ollama.
	DefaultOllamaPort = 11434

	// DefaultOllamaURL is the default URL for Ollama.
	DefaultOllamaURL = "http://localhost:11434"

	// DefaultTemplateTag is the default Docker template image tag.
	DefaultTemplateTag = "claude-code-sandbox-template:warm"

	// StateDirName is the name of the state directory in the user's home.
	StateDirName = ".cloma"

	// WorkspacesDirName is the name of the workspaces subdirectory.
	WorkspacesDirName = "workspaces"
)

// Config holds all configuration values for cloma.
type Config struct {
	// Model is the AI model to use.
	Model string

	// OllamaPort is the port where Ollama is running.
	OllamaPort int

	// OllamaURL is the URL where Ollama is accessible.
	OllamaURL string

	// TemplateTag is the Docker image tag for the sandbox template.
	TemplateTag string

	// StateDir is the path to the state directory (~/.cloma).
	StateDir string

	// WorkspacesDir is the path to the workspaces directory.
	WorkspacesDir string
}

// Initialize sets up default configuration values and environment variable bindings.
// This should be called during application initialization.
func Initialize() error {
	// Set default values
	viper.SetDefault("model", DefaultModel)
	viper.SetDefault("ollama_port", DefaultOllamaPort)
	viper.SetDefault("ollama_url", DefaultOllamaURL)
	viper.SetDefault("template_tag", DefaultTemplateTag)

	// Bind environment variables
	viper.SetEnvPrefix("CLOMA")
	viper.BindEnv("model", "CLOMA_MODEL")

	// Also support OLLAMA_PORT and OLLAMA_URL directly
	viper.BindEnv("ollama_port", "OLLAMA_PORT")
	viper.BindEnv("ollama_url", "OLLAMA_URL")
	viper.BindEnv("template_tag", "CLOMA_TEMPLATE_TAG")

	// Get home directory for state paths
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(home, StateDirName)
	workspacesDir := filepath.Join(stateDir, WorkspacesDirName)

	viper.SetDefault("state_dir", stateDir)
	viper.SetDefault("workspaces_dir", workspacesDir)

	// Bind state directories to environment variables
	viper.BindEnv("state_dir", "CLOMA_STATE_DIR")
	viper.BindEnv("workspaces_dir", "CLOMA_WORKSPACES_DIR")

	return nil
}

// Get returns the current configuration as a Config struct.
func Get() *Config {
	return &Config{
		Model:         viper.GetString("model"),
		OllamaPort:    viper.GetInt("ollama_port"),
		OllamaURL:     viper.GetString("ollama_url"),
		TemplateTag:   viper.GetString("template_tag"),
		StateDir:      viper.GetString("state_dir"),
		WorkspacesDir: viper.GetString("workspaces_dir"),
	}
}

// GetStateDir returns the path to the state directory (~/.cloma).
func GetStateDir() string {
	return viper.GetString("state_dir")
}

// GetWorkspacesDir returns the path to the workspaces directory.
func GetWorkspacesDir() string {
	return viper.GetString("workspaces_dir")
}

// EnsureStateDir creates the state directory if it doesn't exist.
func EnsureStateDir() error {
	stateDir := GetStateDir()
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", stateDir, err)
	}
	return nil
}

// EnsureWorkspacesDir creates the workspaces directory if it doesn't exist.
func EnsureWorkspacesDir() error {
	workspacesDir := GetWorkspacesDir()
	if err := os.MkdirAll(workspacesDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspaces directory %s: %w", workspacesDir, err)
	}
	return nil
}

// EnsureAllDirs creates both state and workspaces directories if they don't exist.
func EnsureAllDirs() error {
	if err := EnsureStateDir(); err != nil {
		return err
	}
	if err := EnsureWorkspacesDir(); err != nil {
		return err
	}
	return nil
}

// GetModel returns the configured model.
func GetModel() string {
	return viper.GetString("model")
}

// GetOllamaPort returns the configured Ollama port.
func GetOllamaPort() int {
	return viper.GetInt("ollama_port")
}

// GetOllamaURL returns the configured Ollama URL.
func GetOllamaURL() string {
	return viper.GetString("ollama_url")
}

// GetTemplateTag returns the configured template tag.
func GetTemplateTag() string {
	return viper.GetString("template_tag")
}