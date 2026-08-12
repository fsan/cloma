#!/usr/bin/env bash
# Entry point script for code agents in Docker sandbox.
#
# This script is copied into the sandbox and executed to start a code agent.
# It supports four agents, selected via the CLOMA_AGENT environment variable:
#
#   - claude (default): Claude Code, driven by the Anthropic Messages API.
#   - grok:             Grok Build, driven by an OpenAI-compatible endpoint.
#   - kimi:             Kimi Code, driven by an OpenAI-compatible endpoint.
#   - openclaw:         OpenClaw, driven by Ollama's native API.
#
# All agents are pointed at an Ollama instance running on the host. Ollama is
# verified with its native API, then the selected agent is launched with a
# model from CLOMA_MODEL and optional extra flags from CLOMA_FLAGS.

set -euo pipefail

# Configuration with defaults (can be overridden via environment)
CLOMA_AGENT="${CLOMA_AGENT:-claude}"
CLOMA_MODEL="${CLOMA_MODEL:-glm-5:cloud}"
CLOMA_OLLAMA_URL="${CLOMA_OLLAMA_URL:-http://host.docker.internal:11434}"
CLOMA_FLAGS="${CLOMA_FLAGS:-}"
WORKSPACE="${WORKSPACE:-$PWD}"

# Make sure the per-user agent install locations are on PATH
# (grok installs to ~/.grok/bin, kimi installs to ~/.kimi-code/bin,
# openclaw installs to ~/.openclaw/bin).
export PATH="${HOME}/.openclaw/bin:${HOME}/.kimi-code/bin:${HOME}/.grok/bin:${HOME}/.local/bin:${PATH}"

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

# Verify Ollama connectivity using its native API (works for both agents).
verify_ollama() {
  log_info "Verifying Ollama connectivity at ${CLOMA_OLLAMA_URL}..."

  local max_attempts=10
  local attempt

  for attempt in $(seq 1 "${max_attempts}"); do
    if curl -fsS "${CLOMA_OLLAMA_URL}/api/tags" >/dev/null 2>&1; then
      log_info "Ollama is reachable"
      return 0
    fi
    log_warn "Attempt ${attempt}/${max_attempts}: Cannot reach Ollama"
    sleep 1
  done

  log_error "Cannot reach Ollama at ${CLOMA_OLLAMA_URL}"
  log_error "Ensure Ollama is running on the host: ollama serve"
  exit 1
}

# Verify model exists in Ollama.
verify_model() {
  log_info "Checking for model: ${CLOMA_MODEL}"

  if curl -fsS -o /dev/null "${CLOMA_OLLAMA_URL}/api/show" -d "{\"model\":\"${CLOMA_MODEL}\"}" 2>/dev/null; then
    log_info "Model ${CLOMA_MODEL} is available"
    return 0
  fi

  log_error "Model ${CLOMA_MODEL} not found in Ollama"
  log_error "Pull it first: ollama pull ${CLOMA_MODEL}"
  exit 1
}

# Print startup information
print_info() {
  local agent_name
  case "${CLOMA_AGENT}" in
    grok)     agent_name="Grok Build" ;;
    kimi)     agent_name="Kimi Code" ;;
    openclaw) agent_name="OpenClaw" ;;
    claude)   agent_name="Claude Code" ;;
    *)        agent_name="${CLOMA_AGENT}" ;;
  esac

  printf '\n'
  printf '===========================================\n'
  printf '     %s - Docker Sandbox\n' "${agent_name}"
  printf '===========================================\n'
  printf '\n'
  printf 'Configuration:\n'
  printf '  Agent:     %s\n' "${agent_name}"
  printf '  Model:     %s\n' "${CLOMA_MODEL}"
  printf '  Ollama:    %s\n' "${CLOMA_OLLAMA_URL}"
  printf '  Workspace: %s\n' "${WORKSPACE}"
  if [ -n "${CLOMA_FLAGS}" ]; then
    printf '  Flags:     %s\n' "${CLOMA_FLAGS}"
  fi
  printf '\n'
}

# Install the selected agent CLI if it is not already available.
ensure_agent_installed() {
  case "${CLOMA_AGENT}" in
    claude)
      if command -v claude >/dev/null 2>&1; then
        return 0
      fi
      log_info "Installing Claude Code..."
      curl -fsSL https://claude.ai/install.sh | bash
      ;;
    grok)
      if command -v grok >/dev/null 2>&1; then
        return 0
      fi
      log_info "Installing Grok Build..."
      curl -fsSL https://x.ai/cli/install.sh | bash
      ;;
    kimi)
      if command -v kimi >/dev/null 2>&1; then
        return 0
      fi
      log_info "Installing Kimi Code..."
      curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash
      ;;
    openclaw)
      if command -v openclaw >/dev/null 2>&1; then
        return 0
      fi
      # OpenClaw requires Node.js 22+; provisioning installs it, but fall back
      # to NodeSource here in case the sandbox was created before this agent
      # was selected.
      need_node=0
      if ! command -v node >/dev/null 2>&1; then
        need_node=1
      else
        node_major="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
        if [ "${node_major}" -lt 22 ]; then
          need_node=1
        fi
      fi
      if [ "${need_node}" -eq 1 ]; then
        log_info "Installing Node.js 22 for OpenClaw..."
        curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
        apt-get install -y --no-install-recommends nodejs
        rm -rf /var/lib/apt/lists/*
      fi
      log_info "Installing OpenClaw..."
      curl -fsSL https://openclaw.ai/install.sh | bash
      ;;
    *)
      log_error "Unknown agent: ${CLOMA_AGENT} (expected 'claude', 'grok', 'kimi' or 'openclaw')"
      exit 1
      ;;
  esac
}

# Write Grok Build's config so it targets the host Ollama via the
# OpenAI-compatible /v1 endpoint. No login is required because the model
# entry carries its own (dummy) API key.
write_grok_config() {
  local config_dir="${HOME}/.grok"
  local config_file="${config_dir}/config.toml"

  mkdir -p "${config_dir}"

  # Use a stable custom-model key so the same value works with `grok -m ollama`.
  cat > "${config_file}" <<EOF
# Generated by cloma. Points Grok Build at the host Ollama instance.
[model.ollama]
model = "${CLOMA_MODEL}"
base_url = "${CLOMA_OLLAMA_URL}/v1"
name = "Ollama (${CLOMA_MODEL})"
api_backend = "chat_completions"
api_key = "ollama"
EOF

  log_info "Wrote grok config to ${config_file}"
}

# Launch Claude Code.
launch_claude() {
  # Claude Code speaks the Anthropic Messages API; Ollama exposes a
  # compatible endpoint at the configured base URL.
  export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_AUTH_TOKEN:-ollama}"
  export ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-}"
  export ANTHROPIC_BASE_URL="${CLOMA_OLLAMA_URL}"

  log_info "Launching Claude Code with model: ${CLOMA_MODEL}"
  printf '\n'

  if [ -n "${CLOMA_FLAGS}" ]; then
    exec claude --model "${CLOMA_MODEL}" ${CLOMA_FLAGS}
  else
    exec claude --model "${CLOMA_MODEL}"
  fi
}

# Launch Grok Build.
launch_grok() {
  # Grok reads its model catalog from ~/.grok/config.toml (written above).
  # A dummy API key satisfies Grok's credential check so it does not prompt
  # for browser login when pointed at a local Ollama instance.
  export XAI_API_KEY="${XAI_API_KEY:-ollama}"

  log_info "Launching Grok Build with model: ollama (${CLOMA_MODEL})"
  printf '\n'

  # The `-m ollama` selects the custom model entry written by write_grok_config.
  if [ -n "${CLOMA_FLAGS}" ]; then
    exec grok -m ollama ${CLOMA_FLAGS}
  else
    exec grok -m ollama
  fi
}

# Node-based agents (kimi, openclaw) use Node fetch (undici), which cannot
# reach the host Ollama through cloma's network proxy: the proxy accepts only
# non-tunneling requests (curl-style), while undici ignores HTTP_PROXY by
# default and tunnels (CONNECT) when forced via NODE_USE_ENV_PROXY, which the
# proxy rejects. The sandbox's localhost:<port> is not directly mapped to the
# host either. So we run a tiny local relay that the agent's fetch talks to
# directly (127.0.0.1), and which forwards to Ollama through the proxy the
# way curl does (non-tunneling). Both kimi and openclaw share this relay:
# the agent selects the API surface via its own base URL (/v1 for kimi's
# OpenAI-compatible client, the native /api for openclaw).
OLLAMA_RELAY_PORT="${OLLAMA_RELAY_PORT:-${KIMI_RELAY_PORT:-18999}}"
OLLAMA_RELAY_UPSTREAM="${OLLAMA_RELAY_UPSTREAM:-${KIMI_RELAY_UPSTREAM:-http://host.docker.internal:11434}}"

# Start the local Ollama relay (idempotent — reuses one already listening).
start_ollama_relay() {
  if ! command -v python3 >/dev/null 2>&1; then
    log_error "python3 is required for the Ollama relay but was not found"
    log_error "re-provision the sandbox: cloma run --agent ${CLOMA_AGENT}"
    exit 1
  fi

  # Reuse a relay that is already listening (e.g. from a previous launch).
  if curl -s --noproxy '*' --max-time 1 \
      "http://127.0.0.1:${OLLAMA_RELAY_PORT}/api/version" >/dev/null 2>&1; then
    log_info "Ollama relay already running on 127.0.0.1:${OLLAMA_RELAY_PORT}"
    return 0
  fi

  local relay="${HOME}/.ollama-relay/relay.py"
  mkdir -p "$(dirname "${relay}")"
  cat > "${relay}" <<'PYEOF'
#!/usr/bin/env python3
"""Local non-tunneling relay: fetch -> 127.0.0.1:PORT -> (HTTP_PROXY) -> Ollama.
Node fetch cannot use cloma's non-tunneling-only proxy; this relay forwards
like curl does, so Node-based agents only ever talk to localhost."""
import http.server, urllib.request, sys, os
UPSTREAM = os.environ.get("OLLAMA_RELAY_UPSTREAM", "http://host.docker.internal:11434")
PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 18999

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def _forward(self):
        n = int(self.headers.get("Content-Length", 0) or 0)
        body = self.rfile.read(n) if n else None
        req = urllib.request.Request(UPSTREAM + self.path, data=body, method=self.command)
        for k, v in self.headers.items():
            if k.lower() in ("host", "content-length", "accept-encoding",
                             "proxy-connection", "connection"):
                continue
            req.add_header(k, v)
        try:
            resp = urllib.request.urlopen(req, timeout=600)
        except urllib.error.HTTPError as e:
            resp = e
        self.send_response(resp.status)
        for k, v in resp.headers.items():
            if k.lower() in ("transfer-encoding", "connection", "content-length"):
                continue
            self.send_header(k, v)
        self.end_headers()
        while True:
            chunk = resp.read(4096)
            if not chunk:
                break
            self.wfile.write(chunk); self.wfile.flush()
    do_GET = _forward
    do_POST = _forward

http.server.ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
PYEOF

  # Detached so it survives `exec` of the agent; dies when the sandbox stops.
  OLLAMA_RELAY_UPSTREAM="${CLOMA_OLLAMA_URL}" setsid python3 "${relay}" "${OLLAMA_RELAY_PORT}" \
    >/tmp/ollama-relay.log 2>&1 </dev/null &

  local i
  for i in $(seq 1 25); do
    if curl -s --noproxy '*' --max-time 1 \
        "http://127.0.0.1:${OLLAMA_RELAY_PORT}/api/version" >/dev/null 2>&1; then
      log_info "Ollama relay listening on 127.0.0.1:${OLLAMA_RELAY_PORT} -> ${CLOMA_OLLAMA_URL}"
      return 0
    fi
    sleep 0.2
  done
  log_error "Ollama relay failed to start (see /tmp/ollama-relay.log)"
  exit 1
}

# Write Kimi Code's config so it targets the local relay (which forwards to
# the host Ollama via the OpenAI-compatible /v1 endpoint). A dummy API key
# satisfies Kimi Code's credential requirement so it does not prompt for
# OAuth login when pointed at a local Ollama instance.
write_kimi_config() {
  local config_dir="${HOME}/.kimi-code"
  local config_file="${config_dir}/config.toml"

  mkdir -p "${config_dir}"

  local kimi_base_url="http://127.0.0.1:${OLLAMA_RELAY_PORT}/v1"

  # The "ollama" model alias selects the custom provider entry below.
  # `default_model` makes `kimi` (with no -m) use Ollama automatically.
  # `secondary_model` points subagents at the same Ollama model so they
  # don't fall back to the built-in default (kimi-k2.7-code:cloud) which
  # does not exist in this Ollama-only setup.
  #
  # When KIMI_SECONDARY_MODEL is set (e.g. to a different Ollama model tag
  # like "kimi-k2.7-code:cloud"), register that model as a separate
  # [models."<tag>"] entry so kimi can resolve it for subagent spawning.
  local secondary_model="ollama"

  if [ -n "${KIMI_SECONDARY_MODEL}" ]; then
    # Verify the secondary model is available in Ollama before registering it.
    if curl -fsS -o /dev/null "${CLOMA_OLLAMA_URL}/api/show" \
        -d "{\"model\":\"${KIMI_SECONDARY_MODEL}\"}" 2>/dev/null; then
      log_info "Secondary model ${KIMI_SECONDARY_MODEL} is available"
    else
      log_error "Secondary model ${KIMI_SECONDARY_MODEL} not found in Ollama"
      log_error "Pull it first: ollama pull ${KIMI_SECONDARY_MODEL}"
      exit 1
    fi
    secondary_model="${KIMI_SECONDARY_MODEL}"
  fi

  cat > "${config_file}" <<EOF
# Generated by cloma. Points Kimi Code at the local relay -> host Ollama.
default_model = "ollama"
default_permission_mode = "manual"

[secondary_model]
model = "${secondary_model}"

[providers.ollama]
type = "openai"
base_url = "${kimi_base_url}"
api_key = "ollama"

[models.ollama]
provider = "ollama"
model = "${CLOMA_MODEL}"
max_context_size = 262144
EOF

  # When a custom secondary model is requested, append a [models."<tag>"]
  # entry pointing at the same Ollama provider. Quoted keys are needed
  # because model tags contain dots (e.g. "kimi-k2.7-code:cloud").
  if [ -n "${KIMI_SECONDARY_MODEL}" ]; then
    cat >> "${config_file}" <<EOF

[models."${KIMI_SECONDARY_MODEL}"]
provider = "ollama"
model = "${KIMI_SECONDARY_MODEL}"
max_context_size = 262144
EOF
  fi

  log_info "Wrote kimi config to ${config_file} (base_url=${kimi_base_url})"
}

# Launch Kimi Code.
launch_kimi() {
  # Kimi reads its providers and models from ~/.kimi-code/config.toml
  # (written above). A dummy API key is embedded in the provider entry so
  # Kimi Code runs against the local Ollama without OAuth login.
  # Disable the update preflight and telemetry so the sandboxed agent does
  # not attempt outbound network calls it cannot reach.
  export KIMI_CODE_NO_AUTO_UPDATE="${KIMI_CODE_NO_AUTO_UPDATE:-1}"
  export KIMI_DISABLE_TELEMETRY="${KIMI_DISABLE_TELEMETRY:-1}"

  # Start the local relay that bridges kimi's fetch to the host Ollama.
  start_ollama_relay

  log_info "Launching Kimi Code with model: ollama (${CLOMA_MODEL})"
  printf '\n'

  # `-m ollama` selects the custom model entry written by write_kimi_config.
  if [ -n "${CLOMA_FLAGS}" ]; then
    exec kimi -m ollama ${CLOMA_FLAGS}
  else
    exec kimi -m ollama
  fi
}

# Write OpenClaw's config so it targets the local relay (which forwards to the
# host Ollama using Ollama's native API). A dummy API key ("ollama-local") is
# accepted by OpenClaw for loopback hosts without real auth. The primary model
# ref is "ollama/<model>" and must match the provider id and a models[] entry.
#
# The agent runs inside an isolated sandbox, so the toolset is configured
# permissively for a coding agent: the "coding" profile (fs, shell, sessions,
# memory, web, agents, media, plugins), web search + web fetch, memory +
# planning, tool-loop safety, forced code mode, subagent session visibility,
# subagent file attachments, shell/exec tuning, image understanding via an
# Ollama vision model, an empty MCP server map ready for user-defined servers,
# and an optional Telegram bot channel.
#
# Environment overrides (pass via cloma --env):
#   OPENCLAW_VISION_MODEL        Ollama vision model for images (default: llava).
#   OPENCLAW_WEB_SEARCH_PROVIDER web search provider (default: ollama, which
#                               reuses the relay; try duckduckgo for keyless).
#   TELEGRAM_BOT_TOKEN           Telegram bot token from @BotFather. When set,
#                               the Telegram channel is enabled and cloma
#                               launches the OpenClaw gateway (which hosts the
#                               bot) instead of the TUI.
#   TELEGRAM_ALLOW_FROM          Comma-separated numeric Telegram user IDs
#                               allowed to DM the bot.
#   TELEGRAM_DM_POLICY           DM policy: pairing (default) | allowlist | open | disabled.
#   TELEGRAM_GROUP_POLICY        Group policy: allowlist (default) | open | disabled.
#   TELEGRAM_GROUP_ALLOW_FROM    Comma-separated numeric user IDs allowed in groups.
#
# Skills: OpenClaw has no non-interactive "install recommended skills" command
# (recommendations are driven by the interactive onboarding/bootstrap flow), so
# none are auto-installed. The skill_workshop tool is available via the coding
# profile; install specific skills with `openclaw skills install @owner/<slug>`.
write_openclaw_config() {
  local config_dir="${HOME}/.openclaw"
  local config_file="${config_dir}/openclaw.json"
  mkdir -p "${config_dir}"

  local relay_base="http://127.0.0.1:${OLLAMA_RELAY_PORT}"

  # The vision model is an Ollama model running on the host; pull it first
  # (e.g. `ollama pull llava`). Override with --env 'OPENCLAW_VISION_MODEL=...'.
  local vision_model="${OPENCLAW_VISION_MODEL:-llava}"

  # Web search defaults to the Ollama provider so it rides the existing relay
  # (host Ollama forwards to Ollama Cloud; needs `ollama signin` on the host).
  # Override with --env 'OPENCLAW_WEB_SEARCH_PROVIDER=duckduckgo' for a keyless
  # provider that reaches the public internet directly from the sandbox.
  local web_search_provider="${OPENCLAW_WEB_SEARCH_PROVIDER:-ollama}"

  # Telegram bot channel — only enabled when a bot token is provided. OpenClaw
  # reads TELEGRAM_BOT_TOKEN as the default-account fallback, so we enable the
  # channel here and let the token resolve from the environment. The bot polls
  # api.telegram.org, so the sandbox needs outbound internet to that host.
  local telegram_json=""
  if [ -n "${TELEGRAM_BOT_TOKEN:-}" ]; then
    local tg='"enabled": true'
    tg="${tg}, \"dmPolicy\": \"${TELEGRAM_DM_POLICY:-pairing}\""
    if [ -n "${TELEGRAM_ALLOW_FROM:-}" ]; then
      tg="${tg}, \"allowFrom\": $(csv_to_json_array "${TELEGRAM_ALLOW_FROM}")"
    fi
    tg="${tg}, \"groupPolicy\": \"${TELEGRAM_GROUP_POLICY:-allowlist}\""
    if [ -n "${TELEGRAM_GROUP_ALLOW_FROM:-}" ]; then
      tg="${tg}, \"groupAllowFrom\": $(csv_to_json_array "${TELEGRAM_GROUP_ALLOW_FROM}")"
    fi
    telegram_json="\"telegram\": { ${tg} }"
    log_info "Telegram channel enabled (dmPolicy=${TELEGRAM_DM_POLICY:-pairing}, groupPolicy=${TELEGRAM_GROUP_POLICY:-allowlist})"
  fi

  cat > "${config_file}" <<EOF
{
  "agents": {
    "defaults": {
      "model": { "primary": "ollama/${CLOMA_MODEL}" }
    }
  },
  "models": {
    "providers": {
      "ollama": {
        "baseUrl": "${relay_base}",
        "apiKey": "ollama-local",
        "api": "ollama",
        "models": [
          { "id": "${CLOMA_MODEL}", "name": "Ollama (${CLOMA_MODEL})", "contextWindow": 262144 },
          { "id": "${vision_model}", "name": "Ollama vision (${vision_model})", "contextWindow": 8192 }
        ]
      }
    }
  },
  "channels": { ${telegram_json} },
  "mcp": {
    "servers": {}
  },
  "tools": {
    "profile": "coding",
    "codeMode": { "enabled": true },
    "loopDetection": { "enabled": true },
    "sessions": { "visibility": "tree" },
    "sessions_spawn": { "attachments": true },
    "exec": {
      "timeoutSeconds": 1800,
      "notifyOnExit": true,
      "commandHighlighting": true,
      "applyPatch": { "enabled": true }
    },
    "web": {
      "search": { "enabled": true, "provider": "${web_search_provider}" },
      "fetch": { "enabled": true }
    },
    "media": {
      "image": { "enabled": true },
      "models": [
        { "provider": "ollama", "model": "${vision_model}", "capabilities": ["image"] }
      ]
    }
  }
}
EOF

  log_info "Wrote openclaw config to ${config_file} (baseUrl=${relay_base}, vision=${vision_model}, webSearch=${web_search_provider})"
}

# csv_to_json_array turns a comma-separated list into a JSON array of strings,
# e.g. "123, 456" -> ["123","456"]. Empty entries are skipped.
csv_to_json_array() {
  local csv="$1" out="[" first=1 item
  local oldIFS="${IFS}"
  IFS=','
  for item in ${csv}; do
    item="${item## }"; item="${item%% }" # trim surrounding spaces
    [ -z "${item}" ] && continue
    if [ "${first}" -eq 1 ]; then first=0; else out="${out}, "; fi
    out="${out}\"${item}\""
  done
  IFS="${oldIFS}"
  printf '%s]' "${out}"
}

# Launch OpenClaw.
launch_openclaw() {
  # OpenClaw reads providers/models from ~/.openclaw/openclaw.json (written
  # above). The relay bridges OpenClaw's Node fetch to the host Ollama.
  start_ollama_relay

  printf '\n'

  # When a Telegram bot token is set, run the OpenClaw gateway — it hosts the
  # Telegram channel (long-polling api.telegram.org) and the agent, so you chat
  # with the agent from Telegram instead of the local TUI.
  if [ -n "${TELEGRAM_BOT_TOKEN:-}" ]; then
    log_info "Launching OpenClaw gateway (Telegram bot) with model: ollama (${CLOMA_MODEL})"
    if [ -n "${CLOMA_FLAGS}" ]; then
      exec openclaw gateway run ${CLOMA_FLAGS}
    else
      exec openclaw gateway run
    fi
  fi

  # Otherwise bare `openclaw` opens the agent TUI against the configured provider.
  log_info "Launching OpenClaw with model: ollama (${CLOMA_MODEL})"
  if [ -n "${CLOMA_FLAGS}" ]; then
    exec openclaw ${CLOMA_FLAGS}
  else
    exec openclaw
  fi
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

  # Make sure the selected agent CLI is present
  ensure_agent_installed

  # Launch the selected agent
  case "${CLOMA_AGENT}" in
    claude)
      launch_claude
      ;;
    grok)
      write_grok_config
      launch_grok
      ;;
    kimi)
      write_kimi_config
      launch_kimi
      ;;
    openclaw)
      write_openclaw_config
      launch_openclaw
      ;;
    *)
      log_error "Unknown agent: ${CLOMA_AGENT} (expected 'claude', 'grok', 'kimi' or 'openclaw')"
      exit 1
      ;;
  esac
}

# Run main
main "$@"