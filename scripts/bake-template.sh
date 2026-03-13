#!/usr/bin/env bash
# Create a warm template image with Claude Code pre-installed

set -euo pipefail

# Source common functions
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

TEMPLATE_SANDBOX_NAME="claude-code-template-bake"
# Use a valid file sharing directory for baking
TEMPLATE_WORKSPACE="${ROOT_DIR}"

printf '=== Baking Warm Template ===\n\n'

# Check prerequisites
require_cmd docker
ensure_sandbox_plugin

# Cleanup handler
cleanup() {
  local exit_code=$?
  if [ $exit_code -ne 0 ]; then
    printf '\nCleaning up failed bake...\n'
  fi
  docker sandbox rm "${TEMPLATE_SANDBOX_NAME}" 2>/dev/null || true
  exit $exit_code
}
trap cleanup EXIT INT TERM

# Remove existing template sandbox if present
printf 'Removing existing template sandbox if present...\n'
docker sandbox rm "${TEMPLATE_SANDBOX_NAME}" 2>/dev/null || true

# Create fresh sandbox using claude agent with project directory as workspace
printf 'Creating sandbox using claude agent...\n'
printf 'Using workspace: %s\n' "${TEMPLATE_WORKSPACE}"
docker sandbox create --name "${TEMPLATE_SANDBOX_NAME}" claude "${TEMPLATE_WORKSPACE}"

printf 'Sandbox created: %s\n' "${TEMPLATE_SANDBOX_NAME}"

# Provision with start script (the claude agent already has Claude Code installed)
printf '\nProvisioning sandbox with start script...\n'
docker sandbox exec \
  --privileged \
  -u root \
  "${TEMPLATE_SANDBOX_NAME}" \
  bash -lc '
    set -euo pipefail

    # Ensure start script directory exists
    install -d -m 0755 /usr/local/bin

    # Create the start script
    cat > /usr/local/bin/start-claude-code.sh << "SCRIPT"
#!/usr/bin/env bash
set -euo pipefail

ANTHROPIC_BASE_URL="${ANTHROPIC_BASE_URL:-http://host.docker.internal:11434}"
CLAUDE_CODE_MODEL="${CLAUDE_CODE_MODEL:-glm-5:cloud}"
CLAUDE_CODE_FLAGS="${CLAUDE_CODE_FLAGS:-}"
WORKSPACE="${WORKSPACE:-$PWD}"

printf "Claude Code Docker Sandbox\n"
printf "Model: %s\n" "${CLAUDE_CODE_MODEL}"
printf "Ollama: %s\n" "${ANTHROPIC_BASE_URL}"
printf "Workspace: %s\n" "${WORKSPACE}"
if [ -n "${CLAUDE_CODE_FLAGS}" ]; then
  printf "Flags: %s\n" "${CLAUDE_CODE_FLAGS}"
fi
printf "\n"

if [ -d "${WORKSPACE}" ]; then
  cd "${WORKSPACE}"
fi

# Launch Claude Code with optional flags
if [ -n "${CLAUDE_CODE_FLAGS}" ]; then
  exec claude --model "${CLAUDE_CODE_MODEL}" ${CLAUDE_CODE_FLAGS}
else
  exec claude --model "${CLAUDE_CODE_MODEL}"
fi
SCRIPT

    chmod +x /usr/local/bin/start-claude-code.sh
    chown -R agent:agent /usr/local/bin/start-claude-code.sh 2>/dev/null || true
  '

# Save as warm template
printf '\nSaving as warm template: %s\n' "${TEMPLATE_TAG}"
docker sandbox save "${TEMPLATE_SANDBOX_NAME}" "${TEMPLATE_TAG}"

# Cleanup
printf '\nCleaning up...\n'
docker sandbox rm "${TEMPLATE_SANDBOX_NAME}" 2>/dev/null || true

printf '\n=== Template Baked Successfully ===\n'
printf 'Template: %s\n' "${TEMPLATE_TAG}"
printf '\nYou can now run: make setup && make run\n'