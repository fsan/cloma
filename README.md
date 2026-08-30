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

# Update to the latest release
cloma update
```

## Menu Bar App (macOS)

cloma ships with an optional Electron app that lives in the macOS
menu bar. It shows all cloma-managed sandboxes and lets you **start**, **stop**,
view **logs**, **delete**, and **force-delete** them without opening a terminal.

### Building and installing

From the repository root:

```bash
# Build the app (produces electron-app/dist/mac-arm64/Cloma.app)
make build-app

# Build and install it to /Applications (or ~/Applications)
make install-app

# Remove build artifacts (dist, generated icons)
make clean-app
```

Or via the cloma CLI:

```bash
# Build and install the app
cloma app install

# Rebuild and reinstall (always rebuilds)
cloma app update

# Remove the app from /Applications
cloma app uninstall

# Remove build artifacts (dist, generated icons)
cloma app clean
```

To remove both the CLI and the app from the system:

```bash
make uninstall
```

The build script downloads the official Electron release directly and assembles
the `.app` bundle by hand — no `npm` or `node_modules` required. The app
shells out to the `cloma` and `docker` CLIs, so both must be on your `PATH`.

### Running from the menu bar

Once installed, launch **Cloma** from Applications. A **“C”** icon
appears in the menu bar. Click it to see your sandboxes and act on them.

### macOS “cannot be opened” / “Launch failed” / Gatekeeper

The bundle is assembled from the official Electron release and then modified
(swapped `Info.plist`, app files, and icon), which invalidates Electron's
original code signature. On Apple Silicon the kernel kills unsigned or
invalidly-signed executables on launch, producing errors like `"Cloma" cannot
be opened` or `Launchd job spawn failed` (POSIX 111).

Two things are required for the bundle to launch:

1. **Intact framework symlinks.** macOS frameworks use a versioned directory
   layout with symlinks (`Versions/Current -> A`, top-level `Resources`,
   `Helpers`, etc. are symlinks into `Versions/Current/`). The build script
   extracts the Electron zip preserving these symlinks; if they are broken
   (e.g. extracted with a tool that writes symlinks as regular text files),
   `codesign` reports "embedded framework contains modified or invalid version"
   and macOS refuses to launch. Run `make clean-app` to clear the cache and
   rebuild if you ever see that error.

2. **Inside-out ad-hoc signing with JIT entitlements.** A simple
   `codesign --force --deep --sign -` is **not sufficient** for Electron on
   Apple Silicon: the V8 engine requires JIT entitlements, and nested
   frameworks/helpers must be signed individually (inside-out). `make install-app`
   and `cloma app install` handle this automatically when run on macOS. If you
   build inside the Linux sandbox and copy the bundle to the host manually, run
   on the host afterwards:

```bash
# Inside-out ad-hoc signing with Electron JIT entitlements:
electron-app/scripts/sign-app.sh "/Applications/Cloma.app"
# Clear the quarantine attribute:
xattr -cr "/Applications/Cloma.app"
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

### Choosing the code agent

By default `cloma run` launches **Claude Code** inside the sandbox. Pass
`--agent grok` to launch **Grok Build** (`grok`), `--agent kimi` to launch
**Kimi Code** (`kimi`), `--agent openclaw` to launch **OpenClaw** (`openclaw`),
`--agent junie` to launch **Junie CLI** (`junie`), or `--agent pi` to launch
the **Pi coding agent** (`pi`) instead. All agents are driven by the same
Ollama instance running on your host.

```bash
# Launch Grok Build (default model)
cloma --agent grok

# Launch Grok Build with a specific model and workspace
cloma --agent grok -w ~/myproject -m glm-4.7-flash

# Launch Kimi Code
cloma --agent kimi

# Launch Kimi Code with a specific model and workspace
cloma --agent kimi -w ~/myproject -m glm-4.7-flash

# Launch OpenClaw
cloma --agent openclaw

# Launch OpenClaw with a specific model and workspace
cloma --agent openclaw -w ~/myproject -m glm-4.7-flash

# Launch Junie CLI
cloma --agent junie

# Launch Junie CLI with a specific model and workspace
cloma --agent junie -w ~/myproject -m glm-4.7-flash

# Launch Pi coding agent
cloma --agent pi

# Launch Pi coding agent with a specific model and workspace
cloma --agent pi -w ~/myproject -m glm-4.7-flash
```

### Interactive mode (`--interactive` / `-i`)

Pass `--interactive` (or `-i`) to `cloma` / `cloma run` to fill in the options
through a wizard instead of flags. Required parameters are asked one at a
time, then the optional ones, and a final confirmation shows the resolved
configuration before anything is created:

```bash
cloma -i
```

```text
=== Cloma interactive setup ===
Press Enter to accept the [default]. Ctrl-C aborts.

Workspace directory [~/myproject]:            ← existing dir, or offer to create
Ollama port [11434]:
Model:                                       ← numbered list of what Ollama
   1) glm-5:cloud (default)                     actually reports on that port
   2) glm-4.7-flash
   3) other (enter a model name)
Select [1-3, default 1]:
Code agent:
   1) Claude Code (default)
   ...
   6) Pi coding agent
Select [1-6, default 1]:

Optional settings (press Enter to accept the default):
Instance name (empty = auto-derived from workspace):
Extra agent flags (empty = none):
Add env var (empty = done):                  ← repeat KEY=VALUE until empty
Use an ephemeral in-memory (tmpfs) workspace instead? [y/N]:

=== Configuration ===
  Agent:     Claude Code (claude)
  ...
Proceed [Y/n]:
```

Details:

- **Flags and config act as defaults** — `-i` combines with them, so
  `cloma -i -m glm-4.7-flash --agent pi` only prompts for what you did not
  already provide.
- **The model list is live** — it is fetched from the Ollama instance on the
  chosen port; a model name entered manually is verified against it. If
  Ollama is not reachable, the model is entered as free text and checked
  later during the preflight.
- **Workspace paths** accept `.`, `~` and relative paths; a path that does
  not exist triggers an offer to create it.
- **Empty input accepts the default** everywhere; Ctrl-C (or declining the
  final `Proceed`) aborts without creating anything.

### Setting environment variables in the sandbox

Pass `--env` (repeatable, `-e` for short) to inject environment variables into
the agent process running inside the sandbox. Each value must be `KEY=VALUE`:

```bash
cloma --flags '--yolo' --model 'kimi-k3:cloud' \
  --env 'KIMI_CODE_EXPERIMENTAL_FLAG=1' \
  --env 'KIMI_CODE_EXPERIMENTAL_SECONDARY_MODEL=1' \
  --env 'KIMI_SECONDARY_MODEL=kimi-k2.7-code:cloud' \
  --agent kimi --name kimi-agent
```

User-supplied variables are applied last, so they can override the cloma-managed
defaults (e.g. `CLOMA_MODEL`) when needed.

When `--agent grok` is used, cloma writes a `~/.grok/config.toml` inside the
sandbox pointing Grok Build at the host Ollama instance (OpenAI-compatible
`/v1` endpoint) and selects the model via `grok -m ollama`. No `grok login` is
required — a dummy API key is written into the model config so Grok Build
runs against the local Ollama without browser authentication.

When `--agent kimi` is used, cloma writes a `~/.kimi-code/config.toml` inside
the sandbox pointing Kimi Code at the host Ollama instance (OpenAI-compatible
`/v1` endpoint, `type = "openai"`) and selects the model via `kimi -m ollama`.
No `kimi login` is required — a dummy API key is embedded in the provider
entry so Kimi Code runs against the local Ollama without OAuth authentication.

By default Kimi Code's subagents use the same Ollama model as the main agent.
Set `KIMI_SECONDARY_MODEL` (via `--env`) to a different Ollama model tag so
spawned subagents use it instead; cloma verifies it exists in Ollama and
registers it in the config. Pull it on the host first
(e.g. `ollama pull kimi-k2.7-code:cloud`).

Kimi Code's OpenAI client (Node `fetch`) cannot use cloma's network proxy,
which accepts only non-tunneling requests (curl-style), while `fetch` ignores
`HTTP_PROXY` and tunnels when forced — the proxy rejects that. cloma
therefore starts a tiny local relay (`~/.ollama-relay/relay.py`, needs
`python3`, installed automatically during provisioning) that Kimi's `fetch`
talks to on `127.0.0.1:18999` and which forwards to the host Ollama through
the proxy the way curl does. The relay is started automatically at launch.
The relay is shared with OpenClaw and can be tuned via `--env`:

| Env var | Default | Description |
|---------|---------|-------------|
| `OLLAMA_RELAY_PORT` | `18999` | Local port the relay listens on (`KIMI_RELAY_PORT` is honored as a legacy fallback) |
| `OLLAMA_RELAY_UPSTREAM` | `http://host.docker.internal:11434` | Where the relay forwards to (`KIMI_RELAY_UPSTREAM` is honored as a legacy fallback) |

When `--agent openclaw` is used, cloma writes a `~/.openclaw/openclaw.json`
inside the sandbox pointing OpenClaw at the host Ollama instance (native
`ollama` API, no `/v1`) and selects the model via the `ollama/<model>` model
ref. No `openclaw onboard` wizard is required. OpenClaw is a Node.js
application and requires Node.js 22+; cloma installs it automatically during
provisioning when missing or outdated. Like Kimi Code, OpenClaw is Node-based
and so uses the same local relay to bridge its `fetch` to the host Ollama
through the non-tunneling proxy.

Because the agent runs in an isolated sandbox, the generated config enables a
permissive, coding-focused toolset:

- **`coding` tool profile** — fs (read/write/edit/apply_patch), shell exec,
  sessions/subagents, memory, web, agents and plugin tools.
- **Web search + web fetch** — see [Web search providers](#web-search-providers)
  below for the provider options, key requirements, and network paths.
- **Memory + planning + loop safety** — `update_plan` (on by default),
  `memory_search/get`, and `tools.loopDetection` enabled.
- **Forced code mode** and **`sessions.visibility: "tree"`** for multi-step
  coding with visible subagents.
- **Subagent file attachments** (`tools.sessions_spawn.attachments`) so spawned
  subagents receive inline file context for multi-file tasks.
- **Shell/exec tuning** (`tools.exec`) — 1800s command timeout, notify on
  background-command exit, command highlighting, and `applyPatch` enabled.
- **Image understanding** via an Ollama vision model (default `llava`; pull it
  on the host with `ollama pull llava`), and an empty **MCP server map** ready
  for servers you define later.
- **Optional Telegram bot** — see [Telegram bot channel](#telegram-bot-channel)
  below; enabled only when you pass a bot token.

> **Skills:** OpenClaw has no non-interactive "install recommended skills"
> command (recommendations come from the interactive onboarding/bootstrap flow),
> so cloma does not auto-install any. The `skill_workshop` tool is available via
> the coding profile; install specific skills with
> `openclaw skills install @owner/<slug>`.

Tune OpenClaw from cloma with `--env` (each `KEY=VALUE`):

| Env var | Default | Description |
|---------|---------|-------------|
| `OPENCLAW_VISION_MODEL` | `llava` | Ollama vision model for image understanding (must be pulled on the host) |
| `OPENCLAW_WEB_SEARCH_PROVIDER` | `ollama` | Web search provider (`ollama`, `duckduckgo`, `brave`, …) |
| `TELEGRAM_BOT_TOKEN` | (unset) | Telegram bot token from @BotFather. When set, enables the Telegram channel and cloma launches the OpenClaw gateway instead of the TUI |
| `TELEGRAM_ALLOW_FROM` | (unset) | Comma-separated numeric Telegram user IDs allowed to DM the bot |
| `TELEGRAM_DM_POLICY` | `pairing` | DM policy: `pairing` \| `allowlist` \| `open` \| `disabled` |
| `TELEGRAM_GROUP_POLICY` | `allowlist` | Group policy: `allowlist` \| `open` \| `disabled` |
| `TELEGRAM_GROUP_ALLOW_FROM` | (unset) | Comma-separated numeric user IDs allowed to trigger the bot in groups |

```bash
# OpenClaw with a different vision model and keyless DuckDuckGo web search
cloma --agent openclaw \
  --env 'OPENCLAW_VISION_MODEL=llama3.2-vision' \
  --env 'OPENCLAW_WEB_SEARCH_PROVIDER=duckduckgo'
```

When `--agent junie` is used, cloma installs the **Junie CLI** (JetBrains) —
specifically the Early Access Program (EAP) build, because pointing Junie at a
local model requires custom model profiles, an EAP-only feature — and writes a
custom model profile to `~/.junie/models/ollama.json` inside the sandbox. The
profile targets the local relay (which forwards to the host Ollama using the
OpenAI-compatible `/v1/chat/completions` endpoint) and is selected with
`junie --model custom:ollama`. Junie has no documented flag or environment
variable for a custom OpenAI-compatible base URL, so a custom model profile is
the only way to drive it from a local Ollama instance; a dummy API key
(`ollama`) satisfies the credential field since Ollama ignores it. Like Kimi
Code and OpenClaw, Junie uses the shared local relay to bridge its HTTP client
to the host Ollama through the non-tunneling proxy, so `python3` is installed
during provisioning.

Tune Junie from cloma with `--env` (each `KEY=VALUE`):

| Env var | Default | Description |
|---------|---------|-------------|
| `JUNIE_FASTER_MODEL` | (unset) | Optional lighter Ollama model for quick tasks (summarization, autocomplete); must be pulled on the host |
| `JUNIE_TEMPERATURE` | `0.3` | Sampling temperature written into the model profile |
| `JUNIE_THEME` | `Dark` | Terminal theme written to `~/.junie/settings.json` (`selectedTheme`): `Dark`, `Light`, `Auto` or `Code` (case-insensitive). cloma defaults to dark because Junie's `Auto` probes the OS/GTK theme and `COLORFGBG`, none of which exist in a headless sandbox, so `Auto` falls back to light. An existing `selectedTheme` (e.g. set via the TUI's `/settings`) is preserved |
| `OLLAMA_RELAY_PORT` | `18999` | Local relay port (`KIMI_RELAY_PORT` is honored as a legacy fallback) |
| `OLLAMA_RELAY_UPSTREAM` | `http://host.docker.internal:11434` | Where the relay forwards to |

```bash
# Junie CLI with a faster sidekick model for quick tasks
cloma --agent junie \
  --env 'JUNIE_FASTER_MODEL=qwen2.5-coder:1.5b'
```

When `--agent pi` is used, cloma installs the **Pi coding agent**
([pi.dev](https://pi.dev)) — a minimal terminal coding harness — via its
official installer (`curl -fsSL https://pi.dev/install.sh | sh`, which runs
non-interactively inside the sandbox) and writes a custom provider to
`~/.pi/agent/models.json` inside the sandbox. The provider targets the local
relay (which forwards to the host Ollama using the OpenAI-compatible `/v1`
chat-completions endpoint) and is selected with `pi --model ollama/<model>`.
Pi has no documented flag or environment variable for a custom base URL, so
`models.json` is the supported way to drive it from a local Ollama instance; a
dummy API key (`ollama`) satisfies Pi's auth-presence requirement since Ollama
ignores it, and the `compat` flags disable the `developer` role and
`reasoning_effort` request fields that Ollama's OpenAI-compatible server
rejects. Pi is a Node.js application (requires Node.js 22.19+ and npm), so
like Kimi Code, OpenClaw and Junie it uses the shared local relay to bridge
its Node fetch to the host Ollama through the non-tunneling proxy; Node.js 22
and `python3` are installed during provisioning. cloma also merges safe
defaults into `~/.pi/agent/settings.json` (`defaultProjectTrust: always` so
the interactive project-trust prompt does not block startup, the Ollama
provider as the default model, install telemetry off) without overwriting
values you set via `/settings`.

Tune Pi from cloma with `--env` (each `KEY=VALUE`):

| Env var | Default | Description |
|---------|---------|-------------|
| `OLLAMA_RELAY_PORT` | `18999` | Local relay port (`KIMI_RELAY_PORT` is honored as a legacy fallback) |
| `OLLAMA_RELAY_UPSTREAM` | `http://host.docker.internal:11434` | Where the relay forwards to |
| `PI_SKIP_VERSION_CHECK` | `1` | Set `0` to let Pi probe pi.dev for updates at startup |

```bash
# Pi coding agent against a specific Ollama model
cloma --agent pi -m glm-4.7-flash
```

#### Web search providers

Web search is enabled by default with `OPENCLAW_WEB_SEARCH_PROVIDER=ollama`.
The provider decides where queries go and whether an API key is required:

| Provider | API key? | Network path from the sandbox | Notes |
|----------|----------|--------------------------------|-------|
| `ollama` (default) | No | sandbox → relay (`127.0.0.1:18999`) → host Ollama → Ollama Cloud | Reuses the existing relay, so no new sandbox egress. Requires `ollama signin` on the host so Ollama can reach Ollama Cloud. |
| `duckduckgo` | **No** | sandbox → `duckduckgo.com` directly | Keyless. Needs the Docker Desktop microVM to have outbound internet egress (cloma's proxy only covers `localhost:11434` → host Ollama). |
| `brave` | Yes (`BRAVE_API_KEY`) | sandbox → Brave API directly | Needs a Brave API key; otherwise set via `--env 'BRAVE_API_KEY=...'`. Same egress caveat as `duckduckgo`. |
| others (`gemini`, `grok`, `kimi`, `perplexity`, `tavily`, `exa`, `searxng`, …) | Varies | sandbox → provider API directly | See the OpenClaw [web search docs](https://docs.openclaw.ai/tools/web); most need a key. |

Keyless providers (`ollama`, `duckduckgo`, `parallel-free`, `codex`) are never
auto-selected by OpenClaw — they must be named explicitly via
`OPENCLAW_WEB_SEARCH_PROVIDER`, which is what the `--env` override does.

```bash
# Keyless, no host setup — DuckDuckGo reaches the internet from the sandbox
cloma --agent openclaw --env 'OPENCLAW_WEB_SEARCH_PROVIDER=duckduckgo'

# Brave with an API key
cloma --agent openclaw \
  --env 'OPENCLAW_WEB_SEARCH_PROVIDER=brave' \
  --env 'BRAVE_API_KEY=YOUR_KEY'
```

> **Note on `ollama` web search:** the relay forwards the request to the host
> Ollama's `/api/web_search` endpoint, which in turn calls Ollama Cloud. If
> the host Ollama isn't signed in (`ollama signin`) or can't reach the
> internet, `ollama` web search fails — in that case switch to `duckduckgo`,
> which needs no host-side credentials.

#### Telegram bot channel

You can drive the sandboxed OpenClaw agent from Telegram by passing a bot
token. When `TELEGRAM_BOT_TOKEN` is set, cloma enables the `channels.telegram`
block in the config and launches the OpenClaw **gateway** (which hosts the bot
via long-polling) instead of the local TUI — so you chat with the agent from
Telegram rather than the terminal.

```bash
# Minimal: just the bot token (DM policy defaults to "pairing" — you approve
# the first user who pairs with the bot)
cloma --agent openclaw --env 'TELEGRAM_BOT_TOKEN=123:abc'

# Lock the bot to specific Telegram users
cloma --agent openclaw \
  --env 'TELEGRAM_BOT_TOKEN=123:abc' \
  --env 'TELEGRAM_ALLOW_FROM=11111111,22222222' \
  --env 'TELEGRAM_DM_POLICY=allowlist'
```

Get a token from [@BotFather](https://t.me/BotFather) (`/newbot`). Find your
numeric user ID via a bot like @userinfobot, or from `openclaw logs --follow`
after messaging the bot. The token resolves in OpenClaw's default account from
the `TELEGRAM_BOT_TOKEN` env var (config would override env; cloma only sets
the env var, so it stays out of the on-disk config).

> **Network:** the bot long-polls `api.telegram.org`, so the sandbox needs
> outbound internet to that host (same egress caveat as `duckduckgo` web
> search — cloma's proxy only covers `localhost:11434` → host Ollama). Only one
> process may poll a given bot token at a time; a `409` conflict means another
> gateway is using the same token.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--workspace` | `-w` | `.` (current dir) | Workspace directory |
| `--model` | `-m` | `glm-5:cloud` | AI model to use |
| `--port` | `-p` | `11434` | Ollama port |
| `--flags` | `-f` | (empty) | Additional agent flags |
| `--agent` | | `claude` | Code agent: `claude` (Claude Code), `grok` (Grok Build), `kimi` (Kimi Code), `openclaw` (OpenClaw), `junie` (Junie CLI) or `pi` (Pi coding agent) |
| `--name` | `-n` | (auto) | Name this cloma instance (overrides the workspace-derived sandbox name) |
| `--env` | `-e` | (empty) | Environment variable to set in the sandbox (`KEY=VALUE`); repeatable |
| `--interactive` | `-i` | off | Fill in options interactively: prompt for required parameters (workspace, port, model, agent), then optional ones |
| `--tempfs` | | off | Use an ephemeral in-memory (tmpfs) workspace on the host instead of the local directory (falls back to a `/tmp` dir on macOS) |
| `--tempfs-size` | | `1g` | Size of the tmpfs workspace (e.g. `1g`, `512m`); Linux tmpfs only |

### Running multiple instances from the same folder

By default the sandbox name is derived from the workspace path, so a given
folder maps to a single sandbox. Pass `--name` (or `-n`) to give an instance
an explicit name, letting you run several agents against the same workspace
without colliding:

```bash
# Two independent instances sharing ~/myproject
cloma --name one   -w ~/myproject --agent claude
cloma --name two   -w ~/myproject --agent kimi

# When --name is set and --workspace is omitted, the current directory is
# used as the workspace (instead of creating a random one), so this runs
# from the folder you are in:
cloma --name one
cloma --name two
```

The value is treated as a **label**: cloma slugifies it (lowercase, hyphens
for special chars) and ensures the `cloma-` prefix, so `--name one` becomes
the sandbox `cloma-one`. Passing an already-prefixed name (e.g. one copied
from `cloma list`) is idempotent. A label with no alphanumeric characters is
rejected.

Use the same `--name` with the other commands to target a specific instance:

```bash
cloma shell  --name one
cloma stop   --name one
cloma clean  --name one   # or: cloma clean one
```

Named instances show up in `cloma list` like any other (`cloma-one`,
`cloma-two`, ...).

### Using an in-memory workspace (`--tempfs`)

Pass `--tempfs` to run the agent against an ephemeral workspace instead of a
local directory, so the agent's file operations never touch your real
filesystem — anything written is lost when the sandbox is removed.

On **Linux** (with root or passwordless `sudo`), cloma mounts a real in-memory
**tmpfs** on the host (under `~/.cloma/tmpfs/<sandbox>`) and bind-mounts that
into the sandbox. On **macOS** (or Linux without the privileges needed to
mount), cloma falls back to a plain empty directory under `/tmp`
(`/tmp/cloma-<sandbox>`) — still ephemeral and isolated from your project
folders, just backed by disk instead of RAM.

```bash
# Run Claude Code in a 1g in-memory workspace (default size; tmpfs on Linux)
cloma --tempfs

# Run OpenClaw in a 512m in-memory workspace, named for easy cleanup
cloma --tempfs --tempfs-size 512m --agent openclaw --name scratch
```

When `--tempfs` is set, `--workspace` is ignored. The sandbox name is taken
from `--name` when given, otherwise a random `cloma-<hash>` name is generated.
`cloma clean` (or a workspace-driven rebuild) unmounts the tmpfs / removes the
temp directory automatically. `--tempfs-size` only applies to the real tmpfs
mount on Linux; it is ignored by the `/tmp` fallback.

### `cloma list`

List all cloma-managed sandboxes. The output includes the sandbox name,
status, agent, creation time, and workspace path.

```bash
# Human-readable output
cloma list

# JSON output for scripting
cloma list --json

# Example output:
# NAME                              STATUS    AGENT    CREATED               WORKSPACE
# --------------------------------------------------------------------------------------------------------------
# cloma-myproject-a1b2c3d4          running   claude   2026-08-18 15:04:05   myproject
# cloma-another-project-e5f6g7h8    stopped   grok     2026-08-18 14:22:10   another-project
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

# Remove by sandbox name directly (bypasses name generation from workspace)
cloma clean --name cloma-myproject-a1b2c3d4
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

### `cloma update`

Update cloma to the latest tagged release. Discovers the latest `vX.Y.Z` tag
from the upstream repository, downloads the source, builds it, and installs
the binary to `/usr/local/bin`. Installing there usually requires root, so the
install step is run with `sudo` automatically when `/usr/local/bin` is not
writable by the current user.

```bash
# Update to the latest release
cloma update

# Only report the latest available tag without installing
cloma update --check

# Install a specific tag instead of the latest
cloma update --version v0.1.0

# Reinstall even when already up to date (or running a dev build)
cloma update --force

# Use a different repository (e.g. a fork)
cloma update --repo https://github.com/yourname/cloma.git
```

### `cloma app`

Manage the macOS menu bar app.

```bash
# Build and install the app
cloma app install

# Rebuild and reinstall (always rebuilds, alias for install --force)
cloma app update

# Rebuild even when already installed
cloma app install --force

# Remove the app from /Applications
cloma app uninstall

# Remove build artifacts (dist, generated icons)
cloma app clean
```

See the [Menu Bar App](#menu-bar-app-macos) section for details.

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

These configure cloma itself (set on the host before running `cloma`, or in
`~/.cloma/config.yaml`). They are distinct from the **agent passthrough**
variables you inject into the sandbox with `--env` — those are documented in
each agent's section:

- OpenClaw: `OPENCLAW_*`, `TELEGRAM_*` — see
  [Tune OpenClaw from cloma](#setting-environment-variables-in-the-sandbox).
- Kimi: `KIMI_SECONDARY_MODEL`, `OLLAMA_RELAY_PORT`, `OLLAMA_RELAY_UPSTREAM` —
  see [the Kimi section](#setting-environment-variables-in-the-sandbox).
- Junie: `JUNIE_FASTER_MODEL`, `JUNIE_TEMPERATURE`, `JUNIE_THEME`,
  `OLLAMA_RELAY_PORT`, `OLLAMA_RELAY_UPSTREAM` — see
  [the Junie section](#setting-environment-variables-in-the-sandbox).
- Pi: `OLLAMA_RELAY_PORT`, `OLLAMA_RELAY_UPSTREAM`, `PI_SKIP_VERSION_CHECK` —
  see [the Pi section](#setting-environment-variables-in-the-sandbox).

| Variable | Description |
|----------|-------------|
| `CLOMA_AGENT` | Code agent to run: `claude` (default), `grok`, `kimi`, `openclaw`, `junie` or `pi` |
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

# Pick the agent via env var instead of --agent
CLOMA_AGENT=openclaw cloma --env 'OPENCLAW_WEB_SEARCH_PROVIDER=duckduckgo'
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
│   ├── cmd/                   # Cobra commands (run, clean, list, doctor, app, ...)
│   ├── sandbox/               # Docker sandbox ops + embedded start-agent.sh
│   ├── workspace/             # Workspace resolution, naming, random, tmpfs
│   ├── ollama/                # Ollama connectivity
│   └── config/                # Configuration
├── electron-app/             # macOS menu bar app (Electron)
│   ├── main.js                # Electron main process (tray, IPC, lifecycle)
│   ├── preload.js             # Context-isolated IPC bridge
│   ├── renderer/              # UI (index.html, logs.html, styles, JS)
│   ├── build-app.sh           # Build + install script
│   └── scripts/generate_icon.py  # Tray/app icon generator
├── image/
│   └── start-agent.sh         # Mirror of internal/sandbox/start-agent.sh
├── go.mod
├── Makefile
└── README.md
```

`internal/sandbox/start-agent.sh` is the agent entry script, embedded into
the binary via `go:embed` and provisioned into each sandbox. `image/start-agent.sh`
is kept as an identical mirror (used when building the warm template image) —
the two must stay in sync.

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
