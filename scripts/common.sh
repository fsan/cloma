#!/usr/bin/env bash

# Shared configuration and utility functions for Claude Code Docker sandbox

set -euo pipefail

# Project root directory
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Configuration with defaults
MODEL="${CLAUDE_CODE_MODEL:-glm-5:cloud}"
OLLAMA_PORT="${OLLAMA_PORT:-11434}"
OLLAMA_URL="http://localhost:${OLLAMA_PORT}"
TEMPLATE_TAG="${CLAUDE_CODE_TEMPLATE_TAG:-claude-code-sandbox-template:warm}"

# Derived values (set by init_context)
SANDBOX_NAME=""
WORKSPACE=""

# Check if a command exists
require_cmd() {
  local cmd="${1:?command is required}"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    printf 'Required command not found: %s\n' "${cmd}" >&2
    exit 1
  fi
}

# Generate a slug from a path (similar to OpenClaw-Docker)
path_to_slug() {
  local path="${1:?path is required}"
  local basename
  basename=$(basename "${path}")
  # Convert to lowercase, replace special chars with hyphens
  printf '%s' "${basename}" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//' | sed 's/-$//'
}

# Generate a hash from workspace path for uniqueness
path_hash() {
  local path="${1:?path is required}"
  printf '%s' "${path}" | shasum -a 256 | cut -c1-8
}

# Initialize sandbox context from workspace path
init_context() {
  local workspace_input="${1:-$PWD}"

  # Resolve to absolute path
  WORKSPACE="$(cd "${workspace_input}" 2>/dev/null && pwd)" || {
    printf 'Workspace path does not exist: %s\n' "${workspace_input}" >&2
    exit 1
  }

  local workspace_slug
  workspace_slug=$(path_slug "${WORKSPACE}")

  local workspace_hash
  workspace_hash=$(path_hash "${WORKSPACE}")

  # Generate unique sandbox name
  SANDBOX_NAME="${CLAUDE_CODE_SANDBOX_NAME:-claude-code-${workspace_slug}-${workspace_hash}}"

  export SANDBOX_NAME WORKSPACE
}

# Alias for path_to_slug (for clarity)
path_slug() {
  path_to_slug "$@"
}

# Wait for Ollama to be available on host
wait_for_ollama() {
  local max_attempts="${OLLAMA_WAIT_ATTEMPTS:-20}"
  local attempt

  printf 'Checking Ollama at %s...\n' "${OLLAMA_URL}"

  for attempt in $(seq 1 "${max_attempts}"); do
    if curl -fsS "${OLLAMA_URL}/api/tags" >/dev/null 2>&1; then
      printf 'Ollama is available.\n'
      return 0
    fi
    printf 'Waiting for Ollama... (%d/%d)\n' "${attempt}" "${max_attempts}"
    sleep 1
  done

  printf 'Unable to reach Ollama at %s.\n' "${OLLAMA_URL}" >&2
  printf 'Ensure Ollama is running: ollama serve\n' >&2
  exit 1
}

# Ensure the specified model exists in Ollama
ensure_model() {
  printf 'Checking for model: %s\n' "${MODEL}"

  if curl -fsS -o /dev/null "${OLLAMA_URL}/api/show" \
    -d "{\"model\":\"${MODEL}\"}" 2>/dev/null; then
    printf 'Model %s is available.\n' "${MODEL}"
    return 0
  fi

  printf 'Model %s not found in Ollama.\n' "${MODEL}" >&2
  printf 'Pull it first: ollama pull %s\n' "${MODEL}" >&2
  exit 1
}

# Ensure Docker Desktop sandbox plugin is available
ensure_sandbox_plugin() {
  if ! docker sandbox version >/dev/null 2>&1; then
    printf 'Docker Desktop sandbox plugin required.\n' >&2
    printf 'Requires Docker Desktop 4.58+\n' >&2
    printf 'Enable sandbox plugin in Docker Desktop settings.\n' >&2
    exit 1
  fi
}

# Check if sandbox exists
sandbox_exists() {
  docker sandbox ls --json 2>/dev/null | grep -q "\"name\":\s*\"${SANDBOX_NAME}\""
}

# Check if sandbox is running
sandbox_running() {
  local sb_status
  sb_status=$(docker sandbox ls --json 2>/dev/null | grep -A1 "\"name\":\s*\"${SANDBOX_NAME}\"" | grep "status" | sed 's/.*"status":\s*"\([^"]*\)".*/\1/')
  [ "${sb_status}" = "running" ]
}

# Ensure sandbox exists (create if needed)
ensure_sandbox() {
  if sandbox_exists; then
    printf 'Sandbox already exists: %s\n' "${SANDBOX_NAME}"
    return 0
  fi

  printf 'Creating sandbox: %s\n' "${SANDBOX_NAME}"

  # Use template if available, otherwise create fresh
  if template_exists; then
    printf 'Using warm template: %s\n' "${TEMPLATE_TAG}"
    docker sandbox create --name "${SANDBOX_NAME}" --load-local-template -t "${TEMPLATE_TAG}" claude "${WORKSPACE}"
  else
    printf 'No warm template found, creating fresh sandbox.\n'
    docker sandbox create --name "${SANDBOX_NAME}" claude "${WORKSPACE}"
    # Provision with Claude Code start script
    provision_sandbox
  fi
}

# Ensure sandbox is running
ensure_sandbox_running() {
  if sandbox_running; then
    printf 'Sandbox is running: %s\n' "${SANDBOX_NAME}"
  else
    # Note: Docker sandbox doesn't have a start command
    # Sandboxes that exist but aren't running need to be recreated
    printf 'Sandbox exists but not running, will recreate: %s\n' "${SANDBOX_NAME}"
    docker sandbox rm "${SANDBOX_NAME}" 2>/dev/null || true
    ensure_sandbox
  fi
}

# Check if template image exists
template_exists() {
  docker image inspect "${TEMPLATE_TAG}" >/dev/null 2>&1
}

# Configure network proxy for sandbox to reach host Ollama
configure_proxy_policy() {
  local port="${1:-${OLLAMA_PORT}}"

  printf 'Configuring network proxy for host port %s...\n' "${port}"

  docker sandbox network proxy "${SANDBOX_NAME}" \
    --allow-host "localhost:${port}" 2>/dev/null || {
    printf 'Warning: Could not configure network proxy.\n' >&2
    printf 'Sandbox may not be able to reach Ollama on host.\n' >&2
  }
}

# Provision sandbox with Claude Code
provision_sandbox() {
  local sandbox_name="${SANDBOX_NAME:-$1}"
  local asset_root="${ROOT_DIR}"
  local claude_version="${CLAUDE_CODE_VERSION:-latest}"

  printf 'Provisioning sandbox with Claude Code...\n'

  docker sandbox exec \
    --privileged \
    -u root \
    -e "CLAUDE_CODE_VERSION=${claude_version}" \
    -e "ASSET_ROOT=${asset_root}" \
    "${sandbox_name}" \
    bash -lc '
      set -euo pipefail
      export DEBIAN_FRONTEND=noninteractive

      # Install dependencies if needed
      if ! command -v curl >/dev/null 2>&1 || ! command -v git >/dev/null 2>&1; then
        apt-get update
        apt-get install -y --no-install-recommends curl ca-certificates git
        rm -rf /var/lib/apt/lists/*
      fi

      # Install Claude Code if not present
      if ! command -v claude >/dev/null 2>&1; then
        curl -fsSL https://claude.ai/install.sh | bash
      fi

      # Ensure start script directory exists
      install -d -m 0755 /usr/local/bin

      # Copy start script if available
      if [ -f "${ASSET_ROOT}/image/start-claude-code.sh" ]; then
        install -m 0755 "${ASSET_ROOT}/image/start-claude-code.sh" /usr/local/bin/start-claude-code.sh
        chown -R agent:agent /usr/local/bin/start-claude-code.sh 2>/dev/null || true
      fi
    '
}

# Cleanup sandbox session on exit
cleanup_sandbox_session() {
  if [ -n "${SANDBOX_NAME}" ] && sandbox_running "${SANDBOX_NAME}"; then
    printf 'Stopping sandbox: %s\n' "${SANDBOX_NAME}"
    docker sandbox stop "${SANDBOX_NAME}" 2>/dev/null || true
  fi
}

# Print version info
print_version() {
  printf 'Claude Code Docker Sandbox\n'
  printf 'Model: %s\n' "${MODEL}"
  printf 'Ollama: %s\n' "${OLLAMA_URL}"
  printf 'Workspace: %s\n' "${WORKSPACE:-not set}"
  printf 'Sandbox: %s\n' "${SANDBOX_NAME:-not set}"
}
