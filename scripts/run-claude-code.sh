#!/usr/bin/env bash
# Run Claude Code in Docker sandbox

set -euo pipefail

# Source common functions
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

init_context "${1:-$PWD}"

# Display configuration
printf '=== Launching Claude Code in Sandbox ===\n\n'
printf 'Model: %s\n' "${MODEL}"
printf 'Ollama: http://host.docker.internal:%s\n' "${OLLAMA_PORT}"
printf 'Workspace: %s\n' "${WORKSPACE}"
printf 'Sandbox: %s\n' "${SANDBOX_NAME}"
if [ -n "${CLAUDE_CODE_FLAGS:-}" ]; then
  printf 'Flags: %s\n' "${CLAUDE_CODE_FLAGS}"
fi
printf '\n'

# Check prerequisites
require_cmd docker
ensure_sandbox_plugin

# Wait for Ollama
wait_for_ollama

# Ensure model exists
ensure_model

# Ensure sandbox exists
ensure_sandbox

# Ensure sandbox is running
ensure_sandbox_running

# Configure network proxy to allow access to host Ollama
configure_proxy_policy "${OLLAMA_PORT}"

# Launch Claude Code interactively
# Note: We use 'docker sandbox exec -it' for interactive terminal mode
printf 'Launching Claude Code...\n\n'

exec docker sandbox exec \
  -it \
  -u agent \
  -w "${WORKSPACE}" \
  -e "ANTHROPIC_AUTH_TOKEN=ollama" \
  -e "ANTHROPIC_API_KEY=" \
  -e "ANTHROPIC_BASE_URL=http://host.docker.internal:${OLLAMA_PORT}" \
  -e "CLAUDE_CODE_MODEL=${MODEL}" \
  -e "CLAUDE_CODE_FLAGS=${CLAUDE_CODE_FLAGS:-}" \
  "${SANDBOX_NAME}" \
  /usr/local/bin/start-claude-code.sh