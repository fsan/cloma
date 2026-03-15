# Cloma - Docker Sandbox Manager

A Go CLI for managing Docker Desktop sandboxes for running code agents in isolation, connecting to Ollama running on the host machine.

## Overview

This project creates a secure, isolated environment for code agents using Docker Desktop's sandbox (microVM) technology. Agents run inside the sandbox while connecting to Ollama on your host machine for inference.

```
┌─────────────────────────────────────────────────────────────┐
│                        Host Machine                          │
│  ┌─────────────────┐    ┌─────────────────────────────────┐ │
│  │     Ollama      │    │          cloma CLI              │ │
│  │  (port 11434)   │    │   run, list, shell, stop, clean │ │
│  └────────┬────────┘    └─────────────────────────────────┘ │
│           │                        │                        │
│           │                        ▼                        │
│           │         ┌──────────────────────────────────┐   │
│           │         │     Docker Sandbox (microVM)     │   │
│           │         │  ┌────────────────────────────┐  │   │
│           └─────────┼──│  Network Proxy (host)      │  │   │
│                     │  │  (allows host:11434 access) │  │   │
│                     │  └────────────────────────────┘  │   │
│                     │              │                    │   │
│                     │              ▼                    │   │
│                     │  ┌────────────────────────────┐  │   │
│                     │  │       Code Agent           │  │   │
│                     │  │  (ANTHROPIC_BASE_URL set)  │  │   │
│                     │  └────────────────────────────┘  │   │
│                     │              │                    │   │
│                     │              ▼                    │   │
│                     │  ┌────────────────────────────┐  │   │
│                     │  │      Workspace             │  │   │
│                     │  │  (git clone repos here)    │  │   │
│                     │  └────────────────────────────┘  │   │
│                     └──────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

1. **Docker Desktop 4.58+** with sandbox plugin enabled
   - Enable sandbox plugin in Docker Desktop settings
2. **Ollama** installed and running on host
   ```bash
   # Install Ollama (if not already installed)
   brew install ollama

   # Start Ollama
   ollama serve
   ```
3. **Model pulled** in Ollama (e.g., glm-5:cloud)
   ```bash
   ollama pull glm-5:cloud
   ```

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/fsan/cloma.git
cd cloma

# Build
make build

# Install to /usr/local/bin (optional)
make install
```

### Using Go Install

```bash
go install github.com/fsan/cloma/cmd/cloma@latest
```

## Quick Start

```bash
# Run in current directory (workspace is auto-mounted)
cloma

# Run with specific workspace
cloma --workspace ~/myproject

# Run with specific model
cloma --model glm-5:cloud

# List all managed sandboxes
cloma list

# Run health checks
cloma doctor
```

## Commands

### `cloma run` (default)

Run an agent in an isolated Docker sandbox.

```bash
# Basic usage - uses current directory as workspace
cloma

# Specify workspace
cloma --workspace /path/to/project

# Specify model
cloma --model glm-4.7-flash

# Pass additional flags to the agent
cloma --flags '--allow-dangerously-skip-permissions'

# Combine options
cloma -w ~/myproject -m glm-4.7-flash --flags '--verbose'
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--workspace` | `-w` | `.` (current dir) | Workspace directory |
| `--model` | `-m` | `glm-5:cloud` | AI model to use |
| `--port` | `-p` | `11434` | Ollama port |
| `--flags` | `-f` | (empty) | Additional agent flags |

### `cloma list`

List all cloma-managed sandboxes.

```bash
# Human-readable output
cloma list

# JSON output for scripting
cloma list --json

# Example output:
# NAME                              STATUS    WORKSPACE
# --------------------------------------------------------------------------------
# cloma-myproject-a1b2c3d4          running   myproject
# cloma-another-project-e5f6g7h8    stopped   another-project
```

### `cloma shell`

Open an interactive shell in the sandbox.

```bash
# Open shell in current workspace's sandbox
cloma shell

# Open shell in specific workspace's sandbox
cloma shell --workspace ~/myproject
```

### `cloma stop`

Stop a running sandbox.

```bash
# Stop current workspace's sandbox
cloma stop

# Stop specific workspace's sandbox
cloma stop --workspace ~/myproject
```

### `cloma clean`

Remove a sandbox completely (stops and removes).

```bash
# Remove with confirmation
cloma clean

# Force removal without confirmation
cloma clean --force

# Remove specific workspace's sandbox
cloma clean --workspace ~/myproject
```

### `cloma doctor`

Run health checks on the system.

```bash
# Human-readable output
cloma doctor

# JSON output
cloma doctor --json

# Example output:
# === Cloma Docker Doctor ===
#
# Checking Docker installation... OK
# Checking Docker Desktop sandbox plugin... OK
# Checking Ollama connectivity... OK
# Checking model glm-5:cloud... OK
# Checking workspace directory... OK
#   /Users/you/myproject
# Checking warm template... WARN
#   Warm template not found: cloma-sandbox-template:warm
#   First run will be slower. Warm templates are optional.
# Checking sandbox... OK
#   cloma-myproject-a1b2c3d4 (stopped)
#
# === Summary ===
# 1 warning(s), 0 error(s)
# Setup is functional but could be improved.
```

### `cloma version`

Print version information.

```bash
cloma version

# JSON output
cloma version --json
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--config` | Config file (default: `~/.cloma/config.yaml`) |
| `-v, --verbose` | Verbose output (stackable: `-v`, `-vv`) |
| `--json` | Output in JSON format |

## Workspace Management

### Automatic Workspace Resolution

`cloma` intelligently resolves workspace paths:

1. **No workspace specified**: Creates a random workspace in `~/.cloma/workspaces/`
   ```bash
   cloma
   # Creates: ~/.cloma/workspaces/cloma-a1b2c3d4/
   # Output: Created new workspace: /Users/you/.cloma/workspaces/cloma-a1b2c3d4
   ```

2. **Current directory (`.`)**: Resolves to absolute path
   ```bash
   cloma --workspace .
   # Uses: /Users/you/current/directory
   ```

3. **Home directory expansion**: Supports `~` and `~/`
   ```bash
   cloma --workspace ~/myproject
   # Uses: /Users/you/myproject
   ```

### Sandbox Naming

Sandboxes are named using the pattern: `cloma-{slug}-{hash}`

- **slug**: Lowercase basename of workspace (special chars replaced with hyphens)
- **hash**: First 8 characters of SHA256 hash of workspace path

Example:
- Workspace: `/Users/fox/myproject`
- Sandbox: `cloma-myproject-bade6fe0`

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `CLOMA_MODEL` | AI model to use (default: `glm-5:cloud`) |
| `OLLAMA_PORT` | Host Ollama port (default: `11434`) |
| `OLLAMA_URL` | Ollama base URL (default: `http://localhost:11434`) |
| `CLOMA_TEMPLATE_TAG` | Template image tag (default: `cloma-sandbox-template:warm`) |
| `CLOMA_STATE_DIR` | State directory (default: `~/.cloma`) |
| `CLOMA_WORKSPACES_DIR` | Workspaces directory (default: `~/.cloma/workspaces`) |

### Example Usage

```bash
# Use a different model
CLOMA_MODEL=glm-4.7-flash cloma

# Use a different Ollama port
OLLAMA_PORT=11435 cloma

# Combine multiple options
CLOMA_MODEL=glm-4.7-flash cloma --workspace ~/myproject
```

## State Directory

All state is stored in `~/.cloma/`:

```
~/.cloma/
├── config.yaml           # Configuration (optional)
└── workspaces/          # Random workspaces created by `cloma`
    ├── cloma-a1b2c3d4/
    └── cloma-e5f6g7h8/
```

## Warm Templates (Optional)

Warm templates pre-install dependencies for faster sandbox startup.

```bash
# Create warm template using Docker
docker build -t cloma-sandbox-template:warm -f Dockerfile.template .
```

## Troubleshooting

### Ollama Not Reachable

```bash
# Check if Ollama is running
curl http://localhost:11434/api/tags

# Start Ollama if not running
ollama serve
```

### Model Not Found

```bash
# List available models
ollama list

# Pull the model
ollama pull glm-5:cloud
```

### Sandbox Plugin Not Available

Ensure Docker Desktop 4.58+ is installed and the sandbox plugin is enabled in settings.

### Connection Issues

```bash
# Run doctor to diagnose
cloma doctor
```

## Development

### Project Structure

```
cloma/
├── cmd/cloma/main.go          # Entry point
├── internal/
│   ├── cmd/                    # Cobra commands
│   ├── sandbox/               # Docker sandbox operations
│   ├── workspace/             # Workspace management
│   ├── ollama/                # Ollama connectivity
│   └── config/                # Configuration
├── image/
│   └── start-agent.sh         # Sandbox entry script
├── go.mod
├── Makefile
└── README.md
```

### Building

```bash
# Build binary
make build

# Run tests
go test ./...

# Install locally
make install
```

## License

GPL v3 - see [LICENSE](LICENSE) for details.
