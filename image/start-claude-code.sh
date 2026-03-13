#!/usr/bin/env bash
# Entry point script for Claude Code in Docker sandbox
# This script is copied into the sandbox and executed to start Claude Code

set -euo pipefail

# Configuration with defaults (can be overridden via environment)
ANTHROPIC_BASE_URL="${ANTHROPIC_BASE_URL:-http://host.docker.internal:11434}"
CLAUDE_CODE_MODEL="${CLAUDE_CODE_MODEL:-glm-5:cloud}"
CLAUDE_CODE_FLAGS="${CLAUDE_CODE_FLAGS:-}"
WORKSPACE="${WORKSPACE:-$PWD}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

log_info() {
  printf '%b[INFO]%b %s\n' "${GREEN}" "${NC}" "$1"
}

log_warn() {
  printf '%b[WARN]%b %s\n' "${YELLOW}" "${NC}" "$1" >&2
}

log_error() {
  printf '%b[ERROR]%b %s\n' "${RED}" "${NC}" "$1" >&2
}

# Verify Ollama connectivity
verify_ollama() {
  log_info "Verifying Ollama connectivity at ${ANTHROPIC_BASE_URL}..."

  local max_attempts=10
  local attempt

  for attempt in $(seq 1 "${max_attempts}"); do
    if curl -fsS "${ANTHROPIC_BASE_URL}/api/tags" >/dev/null 2>&1; then
      log_info "Ollama is reachable"
      return 0
    fi
    log_warn "Attempt ${attempt}/${max_attempts}: Cannot reach Ollama"
    sleep 1
  done

  log_error "Cannot reach Ollama at ${ANTHROPIC_BASE_URL}"
  log_error "Ensure Ollama is running on the host: ollama serve"
  exit 1
}

# Verify model exists
verify_model() {
  log_info "Checking for model: ${CLAUDE_CODE_MODEL}"

  if curl -fsS -o /dev/null "${ANTHROPIC_BASE_URL}/api/show" -d "{\"model\":\"${CLAUDE_CODE_MODEL}\"}" 2>/dev/null; then
    log_info "Model ${CLAUDE_CODE_MODEL} is available"
    return 0
  fi

  log_error "Model ${CLAUDE_CODE_MODEL} not found in Ollama"
  log_error "Pull it first: ollama pull ${CLAUDE_CODE_MODEL}"
  exit 1
}

# Print startup information
print_info() {
  printf '\n'
  printf '===========================================\n'
  printf '     Claude Code - Docker Sandbox\n'
  printf '===========================================\n'
  printf '\n'
  printf 'Configuration:\n'
  printf '  Model:    %s\n' "${CLAUDE_CODE_MODEL}"
  printf '  Ollama:   %s\n' "${ANTHROPIC_BASE_URL}"
  printf '  Workspace: %s\n' "${WORKSPACE}"
  if [ -n "${CLAUDE_CODE_FLAGS}" ]; then
    printf '  Flags:    %s\n' "${CLAUDE_CODE_FLAGS}"
  fi
  printf '\n'
  printf 'Environment:\n'
  printf '  ANTHROPIC_AUTH_TOKEN: %s\n' "${ANTHROPIC_AUTH_TOKEN:-not set}"
  printf '  ANTHROPIC_BASE_URL:   %s\n' "${ANTHROPIC_BASE_URL}"
  printf '\n'
}

# Main entry point
main() {
  print_info

  # Verify connectivity
  verify_ollama
  verify_model

  # Change to workspace directory
  if [ -d "${WORKSPACE}" ]; then
    cd "${WORKSPACE}"
    log_info "Working directory: ${WORKSPACE}"
  else
    log_warn "Workspace does not exist: ${WORKSPACE}, using current directory"
  fi

  # Launch Claude Code
  log_info "Launching Claude Code with model: ${CLAUDE_CODE_MODEL}"
  printf '\n'

  # Build command with optional flags
  if [ -n "${CLAUDE_CODE_FLAGS}" ]; then
    exec claude --model "${CLAUDE_CODE_MODEL}" ${CLAUDE_CODE_FLAGS}
  else
    exec claude --model "${CLAUDE_CODE_MODEL}"
  fi
}

# Run main
main "$@"
