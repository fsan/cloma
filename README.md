# Claude Code Docker Sandbox

A Docker-based sandbox environment for running Claude Code in isolation, connecting to Ollama running on the host machine.

## Overview

This project creates a secure, isolated environment for Claude Code using Docker Desktop's sandbox (microVM) technology. Claude Code runs inside the sandbox while connecting to Ollama on your host machine for inference.

```
┌─────────────────────────────────────────────────────────────┐
│                        Host Machine                          │
│  ┌─────────────────┐    ┌─────────────────────────────────┐ │
│  │     Ollama      │    │         Makefile Targets        │ │
│  │  (port 11434)   │    │  setup, run, shell, stop, clean │ │
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
│                     │  │      Claude Code           │  │   │
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
3. **Model pulled** in Ollama (e.g., qwen3-coder)
   ```bash
   ollama pull qwen3-coder
   ```

## Quick Start

```bash
# 1. Setup (creates warm template)
make setup

# 2. Run Claude Code in sandbox
make run

# 3. When done, stop the sandbox
make stop
```

## Available Commands

| Command | Description |
|---------|-------------|
| `make setup` | Initial setup: check prerequisites, create warm template |
| `make run` | Launch Claude Code in sandbox |
| `make doctor` | Validate setup and connectivity |
| `make shell` | Open interactive shell in sandbox |
| `make logs` | View sandbox logs |
| `make stop` | Stop running sandbox |
| `make clean` | Remove sandbox completely |
| `make template` | Create/bake warm template |
| `make template-clean` | Remove warm template |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL` | `glm-5:cloud` | Ollama model to use |
| `OLLAMA_PORT` | `11434` | Host Ollama port |
| `WORKSPACE` | Current directory | Directory to mount |
| `FLAGS` | *(empty)* | Additional flags for Claude Code (e.g., `--allow-dangerously-skip-permissions`) |
| `CLAUDE_CODE_SANDBOX_NAME` | Auto-generated | Custom sandbox name |
| `CLAUDE_CODE_VERSION` | `latest` | Claude Code version |
| `CLAUDE_CODE_TEMPLATE_TAG` | `claude-code-sandbox-template:warm` | Template image tag |
| `FLAGS` | `` | Additional flags to pass to Claude Code (e.g., `--allow-dangerously-skip-permissions`) |

### Example Usage

```bash
# Use a different model
CLAUDE_CODE_MODEL=glm-4.7-flash make run

# Use a different workspace
WORKSPACE=/path/to/project make run

# Custom Ollama port
OLLAMA_PORT=11435 make run

# Skip permission checks (useful for automation)
FLAGS='--allow-dangerously-skip-permissions' make run

# Combine multiple options
FLAGS='--allow-dangerously-skip-permissions' MODEL=glm-4.7-flash make run
```

## How It Works

### Architecture

1. **Warm Template**: A pre-built Docker image with Claude Code installed for faster startup
2. **Sandbox**: A microVM-based isolated environment created from the template
3. **Network Proxy**: Allows sandbox to connect to host's Ollama via `host.docker.internal`

### Network Configuration

The sandbox connects to Ollama on the host using Docker's `host.docker.internal` DNS name:

- **Host Ollama URL**: `http://host.docker.internal:11434`
- **Network proxy**: Configured automatically via `docker sandbox network proxy`

### Environment Variables Inside Sandbox

Claude Code inside the sandbox sees:

```bash
ANTHROPIC_AUTH_TOKEN=ollama
ANTHROPIC_API_KEY=
ANTHROPIC_BASE_URL=http://host.docker.internal:11434
CLAUDE_CODE_MODEL=qwen3-coder
```

## Development

### Project Structure

```
claude-code-docker/
├── image/
│   └── start-claude-code.sh   # Entry point script for sandbox
├── scripts/
│   ├── common.sh              # Shared functions and configuration
│   ├── setup.sh               # Full setup: checks, template, doctor
│   ├── run-claude-code.sh     # Launch sandbox with Claude Code
│   ├── doctor.sh              # Validate setup connectivity
│   ├── shell.sh               # Interactive shell inside sandbox
│   ├── logs.sh                # View logs
│   ├── stop-sandbox.sh        # Stop running sandbox
│   ├── clean-sandbox.sh       # Remove sandbox
│   ├── bake-template.sh       # Create warm template image
│   └── clean-template.sh      # Remove template image
├── .gitignore
├── Makefile
└── README.md
```

### Adding Custom Configuration

1. Edit `image/start-claude-code.sh` to customize startup behavior
2. Edit `scripts/common.sh` to modify shared functions
3. Run `make template-clean template` to rebuild the template

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
ollama pull qwen3-coder
```

### Sandbox Plugin Not Available

Ensure Docker Desktop 4.58+ is installed and the sandbox plugin is enabled in settings.

### Connection Issues

```bash
# Run doctor to diagnose
make doctor
```

## Sources & References

- [OpenClaw-Docker](https://github.com/SantiaGoMode/OpenClaw-Docker) - Similar architecture for OpenClaw
- [Claude Code Setup](https://code.claude.com/docs/en/setup)
- [Claude Code Third-Party Integrations](https://code.claude.com/docs/en/third-party-integrations)
- [Ollama Claude Code Integration](https://docs.ollama.com/integrations/claude-code)
- [Claude Code Sandboxing](https://code.claude.com/docs/en/sandboxing)

## License

MIT License