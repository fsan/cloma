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

# OpenClaw's web search/fetch tools are served by the Gateway. The bare TUI
# only exposes them when it can connect to a running Gateway; with none
# reachable it falls back to "local embedded" mode where web tools are
# unavailable (the "no web browsing/gateway tools enabled" message). cloma
# starts a loopback-only Gateway in the background before launching the TUI so
# web search/fetch come online. Auth is token mode with a per-sandbox token
# (see ensure_openclaw_gateway_token): "none" mode is rejected by the
# device-pair plugin with "device identity required" even on loopback
# (openclaw/openclaw#75780), while token auth lets the TUI connect as an
# authenticated operator without interactive pairing. --allow-unconfigured
# prevents the Gateway from rewriting the generated config.
OPENCLAW_GATEWAY_PORT="${OPENCLAW_GATEWAY_PORT:-18789}"

# Headless browser env for OpenClaw's browser tool. Chromium was installed to
# /opt/browsers by Playwright during provisioning. --no-sandbox is required in
# Docker: Chromium cannot create its setuid/namespace sandbox inside a
# container (openclaw/openclaw#29879). Headless is forced because the sandbox
# has no display. These are openclaw-specific; other agents ignore them.
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-/opt/browsers}"
export OPENCLAW_BROWSER_NO_SANDBOX="${OPENCLAW_BROWSER_NO_SANDBOX:-1}"
export OPENCLAW_BROWSER_HEADLESS="${OPENCLAW_BROWSER_HEADLESS:-1}"

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

# Provide a stable per-sandbox token for the loopback gateway. Generated once,
# persisted to ~/.openclaw/gateway-token, and reused across launches so a
# still-running gateway (from a previous launch) and a freshly launched TUI
# present the same token. Exported as OPENCLAW_GATEWAY_TOKEN so the gateway
# server and the TUI client both see it; also written into openclaw.json under
# gateway.auth so config-driven clients resolve the same secret.
ensure_openclaw_gateway_token() {
  local token_file="${HOME}/.openclaw/gateway-token"
  if [ -s "${token_file}" ]; then
    OPENCLAW_GATEWAY_TOKEN="$(cat "${token_file}" 2>/dev/null)"
  fi
  if [ -z "${OPENCLAW_GATEWAY_TOKEN:-}" ]; then
    OPENCLAW_GATEWAY_TOKEN="$(head -c 32 /dev/urandom | base64 | tr -d '=+/' | cut -c1-32)"
    mkdir -p "$(dirname "${token_file}")"
    printf '%s' "${OPENCLAW_GATEWAY_TOKEN}" > "${token_file}"
    chmod 600 "${token_file}"
  fi
  export OPENCLAW_GATEWAY_TOKEN
}

# Start the OpenClaw Gateway on loopback (idempotent — reuses one already
# listening). The Gateway provides web search/fetch and other gateway tools to
# the TUI; without it the TUI runs in local-embedded mode with no web tools.
# Detached so it survives `exec` of the TUI; dies when the sandbox stops.
# Non-fatal: if it cannot bind, the TUI still launches in local-embedded mode
# (just without web tools) so the agent remains usable.
start_openclaw_gateway() {
  # A stable token is required: the TUI (this shell) and a possibly already
  # running gateway (from a previous launch) must present the same token.
  ensure_openclaw_gateway_token

  # Reuse a gateway that is already listening (e.g. from a previous launch).
  if curl -s --noproxy '*' --max-time 1 \
      "http://127.0.0.1:${OPENCLAW_GATEWAY_PORT}/" >/dev/null 2>&1; then
    log_info "OpenClaw gateway already running on 127.0.0.1:${OPENCLAW_GATEWAY_PORT}"
    return 0
  fi

  log_info "Starting OpenClaw gateway on 127.0.0.1:${OPENCLAW_GATEWAY_PORT}..."

  setsid openclaw gateway \
      --allow-unconfigured --auth token --bind loopback \
      --port "${OPENCLAW_GATEWAY_PORT}" \
    >/tmp/openclaw-gateway.log 2>&1 </dev/null &

  local i
  for i in $(seq 1 50); do
    if curl -s --noproxy '*' --max-time 1 \
        "http://127.0.0.1:${OPENCLAW_GATEWAY_PORT}/" >/dev/null 2>&1; then
      log_info "OpenClaw gateway listening on 127.0.0.1:${OPENCLAW_GATEWAY_PORT}"
      return 0
    fi
    sleep 0.2
  done
  log_warn "OpenClaw gateway did not become reachable (see /tmp/openclaw-gateway.log); web tools may be unavailable"
  return 0
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
# planning, tool-loop safety, codeMode disabled (direct tool exposure — the
# agent sees the real shell exec, browser, and web_search tools directly,
# instead of code mode's QuickJS JS sandbox which hides the full catalog behind
# a JS bridge that small/local models can't drive; this OpenClaw build's schema
# only accepts a boolean, so "auto" is not available here), subagent session
# visibility, subagent file attachments, shell/exec with full
# approval mode + gateway host (so the agent can install dependencies and write
# code without an approval gate), a headless Chromium for the browser tool
# (installed by Playwright during provisioning, --no-sandbox for Docker), image
# understanding via an Ollama vision model, an empty MCP server map ready for
# user-defined servers, and an optional Telegram bot channel.
#
# Environment overrides (pass via cloma --env):
#   OPENCLAW_CODE_MODE           1 enables code mode (JS-sandbox tool orchestration)
#                               for capable models; default 0 = direct tools, which
#                               is what small/local Ollama models need.
#   OPENCLAW_VISION_MODEL        Ollama vision model for images (default: llava).
#   OPENCLAW_WEB_SEARCH_PROVIDER web search provider (default: ollama, which
#                               reuses the relay; try duckduckgo for keyless).
#   OPENCLAW_BROWSER_HEADLESS    Force headless off with 0 (default 1; the
#                               sandbox has no display).
#   OPENCLAW_BROWSER_NO_SANDBOX  1 (default) adds --no-sandbox to Chromium,
#                               required in Docker; set 0 only with privileges.
#   PLAYWRIGHT_BROWSERS_PATH      Where Chromium lives (default /opt/browsers).
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

  # Resolve the loopback gateway token before writing config so it can be
  # embedded in gateway.auth below (shared by the gateway server and the TUI).
  ensure_openclaw_gateway_token

  local relay_base="http://127.0.0.1:${OLLAMA_RELAY_PORT}"

  # The vision model is an Ollama model running on the host; pull it first
  # (e.g. `ollama pull llava`). Override with --env 'OPENCLAW_VISION_MODEL=...'.
  local vision_model="${OPENCLAW_VISION_MODEL:-llava}"

  # Web search defaults to the Ollama provider so it rides the existing relay
  # (host Ollama forwards to Ollama Cloud; needs `ollama signin` on the host).
  # Override with --env 'OPENCLAW_WEB_SEARCH_PROVIDER=duckduckgo' for a keyless
  # provider that reaches the public internet directly from the sandbox.
  local web_search_provider="${OPENCLAW_WEB_SEARCH_PROVIDER:-ollama}"

  # Code mode (tools.codeMode) hides the full tool catalog behind a QuickJS JS
  # sandbox: the agent only gets a JS `exec` + `wait` and must drive the real
  # tools (shell, browser, web) through a tools.search()/tools.call() bridge.
  # It's a token-saving optimization for "preferred" models (Claude/GPT/Gemini
  # tier) that reliably write that JS, but it traps small/local Ollama models
  # that can't — they lose direct shell/browser/web access (the "I can only run
  # JS / can't install deps" failure). This OpenClaw build's schema only accepts
  # a boolean (no "auto" per-model switch), so cloma defaults it OFF (direct
  # tools work for every model). Override with --env 'OPENCLAW_CODE_MODE=1' when
  # running a capable model that benefits from code mode.
  local code_mode="false"
  case "${OPENCLAW_CODE_MODE:-0}" in
    1|true|TRUE|True|yes) code_mode="true" ;;
    *) code_mode="false" ;;
  esac

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

  # Locate the headless Chromium installed by Playwright during provisioning
  # (arm64-safe, unlike the amd64-only google-chrome .deb). Fall back to any
  # system Chrome/Chromium. When a binary is found, pin it via executablePath
  # so OpenClaw's browser plugin launches headless with --no-sandbox (required
  # in Docker). noSandbox/headless are set unconditionally below.
  local browser_path=""
  local p
  for p in /opt/browsers/chromium-*/chrome-linux/chrome \
           /opt/browsers/chromium-*/chrome-linux/headless_shell; do
    if [ -x "$p" ]; then browser_path="$p"; break; fi
  done
  if [ -z "${browser_path}" ]; then
    for p in /usr/bin/google-chrome-stable /usr/bin/chromium /usr/bin/chromium-browser; do
      if [ -x "$p" ]; then browser_path="$p"; break; fi
    done
  fi
  local browser_json='"enabled": true, "headless": true, "noSandbox": true'
  if [ -n "${browser_path}" ]; then
    browser_json="${browser_json}, \"executablePath\": \"${browser_path}\""
    log_info "Browser: ${browser_path}"
  else
    log_warn "No Chromium found; browser tool will try to auto-install on first use (slow) or fail"
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
          { "id": "${CLOMA_MODEL}", "name": "Ollama (${CLOMA_MODEL})", "contextWindow": 262144, "params": { "num_ctx": 262144 } },
          { "id": "${vision_model}", "name": "Ollama vision (${vision_model})", "contextWindow": 8192, "params": { "num_ctx": 8192 } }
        ]
      }
    }
  },
  "channels": { ${telegram_json} },
  "mcp": {
    "servers": {}
  },
  "gateway": {
    "auth": { "mode": "token", "token": "${OPENCLAW_GATEWAY_TOKEN}" }
  },
  "browser": { ${browser_json} },
  "tools": {
    "profile": "coding",
    "codeMode": { "enabled": ${code_mode} },
    "loopDetection": { "enabled": true },
    "sessions": { "visibility": "tree" },
    "sessions_spawn": {
      "attachments": {
        "enabled": true,
        "maxTotalBytes": 5242880,
        "maxFiles": 50,
        "maxFileBytes": 1048576
      }
    },
    "exec": {
      "mode": "full",
      "host": "gateway",
      "timeoutSec": 1800,
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
# ensure_openclaw_bootstrap writes OpenClaw's persona bootstrap file (SOUL.md)
# into the workspace before launch. OpenClaw loads workspace bootstrap files
# (SOUL.md/USER.md/IDENTITY.md/HEARTBEAT.md) into the agent's context at startup
# (controlled by agents.defaults.contextInjection). By default OpenClaw creates
# these if missing, so writing SOUL.md first makes the agent load our directive
# instead of an auto-generated one. This is the supported, schema-stable way to
# steer the agent's answer style without guessing config keys (which the schema
# rejects, e.g. there is no agents.defaults.instructions/systemPrompt field and
# promptOverlays is GPT-5-only). Set OPENCLAW_NO_BOOTSTRAP=1 to skip; an existing
# SOUL.md is left untouched so user-authored personas win.
ensure_openclaw_bootstrap() {
  case "${OPENCLAW_NO_BOOTSTRAP:-0}" in
    1|true|TRUE|True|yes) return 0 ;;
  esac
  local soul="SOUL.md"
  if [ -s "${soul}" ]; then
    log_info "Existing ${soul} found; leaving it in place"
    return 0
  fi
  log_info "Writing ${soul} (answer-style directive) into workspace"
  cat > "${soul}" <<'SOUL'
# Soul

You are an autonomous coding agent running in an isolated sandbox. You have
real tools: a shell (exec), a headless browser, and web search/fetch. You can
install dependencies — pip, apt-get and npm all work; plain `pip install <pkg>`
is fine. `python` is available (alias of python3). You have passwordless sudo.

## Autonomy — important
- Complete the task by whatever means necessary. Make decisions, pick tools,
  install software, write/run/edit files, browse, and search without asking
  for permission at each step.
- Do not stop to ask "should I?" for ordinary work. Proceed and get it done.
- Do not stop until the goal is fully achieved. If one approach fails, rethink
  the problem and try a different approach — keep going. After each reply, if
  the goal is not yet complete, continue working toward it in the same turn
  instead of stopping and waiting for the user. Only stop when the goal is
  done, or when you hit a genuine hard blocker (safety, or a credential /
  permission you cannot obtain yourself).
- Only stop and ask the user when an action is genuinely dangerous or unsafe:
  destroying data, irreversible deletions outside your task scope, exposing
  secrets/credentials, or anything that could break the environment. When in
  doubt about safety (not difficulty), pause and ask — difficulty is never a
  reason to stop.
- Never hand a task back to the user. Do NOT say "you can check ...", "I
  recommend you visit ...", or "would you like me to fetch/browse ...?" — if a
  tool (web fetch, browser, exec) can do it, USE the tool and report the
  result. Asking "would you like me to...?" is always wrong: just do it. Only
  ask the user when you need a decision or a credential you cannot obtain
  yourself.
- If a path is blocked, find another way yourself; only report a hard blocker
  after you have genuinely tried alternatives.

## Answer style — important
- Answer the user's question directly and concisely. Lead with the result.
- If you ran code or a shell command, put the actual output/result as the
  FIRST line of your reply.
- Do NOT open with "Done. Here's what happened:" or a numbered recap of steps.
- Do not send progress narration as separate messages ("I'll install...",
  "Now running the script:"). Do the work silently, then reply with the
  answer.
- Only recap the steps you took if the user explicitly asks.
- No filler, no apologies, no "I'm sorry, I cannot...". If something failed,
  say the error and the fix in one line.
SOUL
}

launch_openclaw() {
  # OpenClaw reads providers/models from ~/.openclaw/openclaw.json (written
  # above). The relay bridges OpenClaw's Node fetch to the host Ollama.
  start_ollama_relay
  ensure_openclaw_bootstrap

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

  # Otherwise bare `openclaw` opens the agent TUI against the configured
  # provider. Start a loopback Gateway first so the TUI connects to it and
  # web search/fetch (gateway tools) are available; without a Gateway the TUI
  # runs in local-embedded mode with no web tools.
  start_openclaw_gateway

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