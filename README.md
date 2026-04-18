# PookiePaws

![PookiePaws banner](./assets/pookiepaws.svg)

[![Release](https://img.shields.io/github/v/release/mitpoai/pookiepaws?display_name=tag)](https://github.com/mitpoai/pookiepaws/releases)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/mitpoai/pookiepaws)](./LICENSE)

> Local-first marketing operations runtime from MITPO for research, strategy, approvals, file access control, and campaign execution.

PookiePaws is a pure-Go, stdlib-first marketing automation runtime built around one binary, a local operator console, approval-aware external actions, approval-gated workspace file access, and a strict host-side secret boundary. It is intentionally narrower than a general-purpose AI workspace: the product is optimized for marketing operators who need clarity, auditability, and predictable workflow execution.

## Why PookiePaws

- It is local-first and inspectable instead of relying on a heavy hosted control plane.
- It keeps workflow execution visible through a spatial operator console and event stream.
- It defaults to approval-gated outbound CRM, SMS, and workspace file actions.
- It supports local LLMs through an OpenAI-compatible boundary, so you can run a brain without a cloud API key.
- It keeps secrets host-side in `.security.json` and avoids putting provider credentials into prompts.
- It is designed as a marketing operations runtime, not a generic shell agent.

## What Makes It Different

- Small, single-binary runtime rather than a broad multi-service agent platform
- Operator-first console rather than a chat-only interface
- Workflow templates and direct forms for real marketing tasks
- Approval queue with human-readable action summaries
- File-permission queue for explicit workspace reads and writes
- Event-driven audit trail exposed through SSE and persisted runtime state
- OpenAI-compatible LLM boundary without forcing a specific provider

## Current Capabilities

- Non-blocking `EventBus` and goroutine-based `SubTurnManager`
- Local operator console served from the same binary
- Direct workflow forms for:
  - UTM validation
  - CRM lead routing
  - SMS draft creation
  - WhatsApp draft creation
- Optional brain routing from free-text to structured workflow commands
- Live HTTP adapters for SALESmanago and Mitto
- WhatsApp outbound channel adapter with approval-gated sends and delivery receipts
- Approval-gated workspace reads and writes through `PermissionedSandbox`
- Runtime state and audit records stored under `~/.pookiepaws/`

## Installation

### macOS / Linux

```bash
brew install mitpoai/pookiepaws/pookie
```

Or use the install script directly:

```bash
curl -fsSL https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.sh | bash
```

### Windows

```powershell
winget install MITPOAI.PookiePaws
```

Or use PowerShell:

```powershell
irm https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.ps1 | iex
```

### From source

```bash
go install github.com/mitpoai/pookiepaws/cmd/pookie@latest
```

### Manual download

Download the binary for your platform from the [Releases page](https://github.com/MITPOAI/PookiePaws/releases/latest):

| Platform | File |
|----------|------|
| Windows 64-bit | `pookie_<version>_windows_amd64.zip` |
| macOS Apple Silicon | `pookie_<version>_darwin_arm64.tar.gz` |
| macOS Intel | `pookie_<version>_darwin_amd64.tar.gz` |
| Linux 64-bit | `pookie_<version>_linux_amd64.tar.gz` |
| Linux ARM | `pookie_<version>_linux_arm64.tar.gz` |

The installer scripts automatically detect your OS and architecture, download the right binary, and add `pookie` to your PATH. No Go toolchain required.

### Shell completion

```bash
pookie completion bash > /etc/bash_completion.d/pookie    # then re-source
pookie completion zsh  > "${fpath[1]}/_pookie"            # zsh
pookie completion fish > ~/.config/fish/completions/pookie.fish
pookie completion powershell >> $PROFILE                  # Windows
```

## Updating

| Channel        | Command                                          |
|----------------|--------------------------------------------------|
| Homebrew tap   | `brew upgrade mitpoai/pookiepaws/pookie`         |
| WinGet         | `winget upgrade MITPOAI.PookiePaws`              |
| Install script | re-run `install.sh` or `install.ps1`             |

`pookie version --check` performs a live lookup against GitHub Releases.
A short notice on stderr also appears during interactive commands when an
update is available; opt out with `POOKIEPAWS_NO_UPDATE_NOTIFIER=1`
(see the "Update Notifications" section).

## Quick Start

1. Install `pookie`.

Use a release binary from the [latest release](https://github.com/MITPOAI/PookiePaws/releases/latest), the installer scripts above, or build it locally if you need a development build:

```powershell
go build -o pookie.exe ./cmd/pookie
```

2. Run the interactive setup wizard.

```powershell
.\pookie.exe init
```

The new wizard uses arrow keys for the brain provider and model menus, masked input for secrets, connectivity checks for the selected LLM provider, and a checkbox flow for optional channels:

- Brain providers: OpenAI, Anthropic, Google, OpenRouter, Ollama, LM Studio / Local
- Marketing channels: Meta WhatsApp, Mitto SMS, SALESmanago

Credentials are written atomically to `~/.pookiepaws/.security.json` by default. The wizard uses `os.UserHomeDir()` plus `filepath.Join()` internally, and keeps `--home` / `POOKIEPAWS_HOME` support for custom runtime roots. On Unix systems the file is locked to `0600`; on Windows the save remains best-effort because ACLs do not map directly to Unix mode bits.

3. Start the agent.

```powershell
.\pookie.exe start
```

Open [http://127.0.0.1:18800/](http://127.0.0.1:18800/).

## Setup Wizard

`pookie init` is now a staged setup wizard rather than a line-by-line prompt list.

**Controls**

- `↑/↓` move through menus
- `Space` toggle channel checkboxes
- `Enter` confirm
- `Esc` go back or cancel the current wizard step
- `Ctrl+C` exit immediately

**Brain setup**

- Hosted presets auto-fill the exact chat-completions endpoint and curated model IDs.
- Local presets do not require an API key.
- LM Studio attempts local model discovery from `/v1/models` and falls back to manual model entry when the server is offline.
- After you enter a hosted API key, Pookie runs a short connectivity check before saving the config.

**Channel setup**

- Channels are optional and selected through a checkbox screen.
- WhatsApp runs a real credential test against the configured Meta-compatible endpoint.
- Mitto and SALESmanago currently use strict field and URL validation during setup.
- A final review screen shows the selected brain, active channels, redacted secrets, and the config path before the file is written.

## Cross-Platform Build & Run

PookiePaws compiles to a single self-contained binary with no runtime
dependencies. Use the `GOOS` and `GOARCH` environment variables to
cross-compile from any machine.

### Windows (64-bit)

```powershell
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -ldflags="-s -w" -o pookie.exe ./cmd/pookie
```

### macOS — Apple Silicon (arm64)

```bash
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o pookie-darwin-arm64 ./cmd/pookie
```

### macOS — Intel (amd64)

```bash
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o pookie-darwin-amd64 ./cmd/pookie
```

### Linux (amd64) — for Hetzner, Fly.io, and similar hosts

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o pookie-linux-amd64 ./cmd/pookie
```

### Linux (arm64) — for Raspberry Pi or ARM-based cloud VMs

```bash
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o pookie-linux-arm64 ./cmd/pookie
```

The `-ldflags="-s -w"` strips the symbol table and DWARF debug info, reducing
the binary size significantly. Drop those flags if you need a debuggable build.

> **Deploying to Fly.io or Hetzner:** copy the Linux binary to your server,
> run `pookie init` once to create the secrets file, then start the agent with
> `./pookie-linux-amd64 start --addr 0.0.0.0:18800`.

## Format, Vet, Test, And Build

The repository root does not contain Go source files, so commands such as `go vet` must be run with package patterns like `./...` or explicit package paths.

```powershell
Get-ChildItem -Path cmd,internal -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go vet ./...
go test ./...
go build -v ./cmd/pookie/...
go build -o pookie.exe ./cmd/pookie
```

## Verify The Console And Status

```powershell
# Using the CLI
.\pookie.exe status

# Or directly via the REST API
Invoke-RestMethod http://127.0.0.1:18800/api/v1/console
Invoke-RestMethod http://127.0.0.1:18800/api/v1/status
Invoke-RestMethod http://127.0.0.1:18800/healthz
Invoke-RestMethod http://127.0.0.1:18800/readyz
Invoke-RestMethod http://127.0.0.1:18800/api/v1/skills
Invoke-RestMethod http://127.0.0.1:18800/api/v1/diagnostics
```

## CLI Commands

```
pookie                        Launch interactive menu (arrow keys)
pookie start                  Run the daemon + web console (with research scheduler)
pookie chat                   Talk to Pookie in your terminal (AI mode)
pookie list                   Show all installed marketing skills
pookie run <skill>            Execute a skill directly in this terminal
pookie status                 Check whether the agent is running
pookie research <sub>         Watchlists, schedule, refresh, status, recommendations
pookie sessions               Inspect persisted control-plane sessions
pookie approvals              Review or resolve pending approvals
pookie audit                  Tail recent audit events from local state
pookie doctor                 Print local runtime diagnostics
pookie smoke                  Run operator smoke checks
pookie context                Inspect prompt, memory, variables
pookie memory                 Manage persistent brain memory
pookie install <repo>         Install a skill from GitHub
pookie init                   First-run setup wizard
pookie version [--check]      Print version; --check forces live release lookup
pookie -v, --version          Aliases for `pookie version`
pookie -h, --help             Show help
```

**Interactive menu:** Running `pookie` with no arguments opens an arrow-key
selection menu where you can start the daemon, chat with Pookie, list skills,
or run a specific skill — no command memorisation required.

**Chat REPL:** `pookie chat` opens a conversational terminal session. Type
marketing goals in plain English and Pookie picks the best skill. Built-in
commands: `/skills`, `/clear`, `/exit`.

**Run a skill from the terminal:**

```powershell
.\pookie.exe run utm-validator --input url="https://example.com?utm_source=nl&utm_medium=email&utm_campaign=launch"
```

**Install a community skill:**

```powershell
.\pookie.exe install owner/pookiepaws-skill-ga4-audit
# or at a specific ref:
.\pookie.exe install owner/repo@v1.2.0
```

## Update Notifications

Interactive commands (`pookie`, `start`, `chat`, `init`, `list`, `doctor`) print a short two-line stderr notice when a newer GitHub release has been seen recently — the check runs in the background using a cached result and never blocks startup.

`pookie version` prints the running build along with the cached upgrade hint, while `pookie version --check` bypasses the cache and performs a live lookup against the GitHub Releases API:

```bash
pookie version
```

```bash
pookie version --check
```

**Opt out.** Set `POOKIEPAWS_NO_UPDATE_NOTIFIER=1` to suppress the notice entirely. The notifier also auto-disables when `CI=1` is set, matching the `gh` and `npm` convention so CI logs stay clean.

```bash
export POOKIEPAWS_NO_UPDATE_NOTIFIER=1
# or per-invocation
POOKIEPAWS_NO_UPDATE_NOTIFIER=1 pookie start
```

```powershell
$env:POOKIEPAWS_NO_UPDATE_NOTIFIER = "1"
# persist for new shells
setx POOKIEPAWS_NO_UPDATE_NOTIFIER 1
```

**Cache location.** The notifier caches the latest-release lookup at `os.UserCacheDir()/pookiepaws/update-check.json` with a 24-hour TTL and atomic writes. Set `POOKIEPAWS_UPDATE_CACHE_PATH` to redirect the cache file (used by tests, but also a documented escape hatch for sandboxed environments).

**Upgrade hint.** When a newer release is detected, the printed hint prefers `winget` or `brew` if either is on `PATH`; otherwise it points back at the install scripts (`install.sh` / `install.ps1`).

## Logging

Operational logs from the CLI go through `log/slog` and are written to stderr. Two env vars control the output:

- `POOKIEPAWS_LOG_FORMAT`: `text` (default, human-friendly) or `json` for structured shipping
- `POOKIEPAWS_LOG_LEVEL`: `debug`, `info` (default), `warn`, `error`

User-facing CLI output (the pretty box renderer, banners, command results) is unaffected — only operational warnings, errors, and scheduler events route through slog.

## Research Automation

`pookie research` manages watchlists, dossier generation, and the periodic refresh scheduler. The scheduler runs *only* inside `pookie start` — one-shot commands (`run`, `version`, `doctor`) never trigger it.

```bash
# Configure the schedule (manual is the default — pick hourly or daily to enable)
pookie research schedule --mode hourly

# Replace your watchlists from a JSON file
pookie research watchlists apply --file watchlists.json

# Or pipe from stdin
cat watchlists.json | pookie research watchlists apply --stdin

# Submit a refresh right now (mirrors what the scheduler does on its tick)
pookie research refresh

# Inspect scheduler state
pookie research status

# Browse recommendations produced by recent dossiers
pookie research recommendations --status draft
```

**Schedule modes.** `manual` (default; the scheduler logs and idles), `hourly` (60-minute minimum gap between runs), `daily` (24-hour minimum gap). The scheduler skips if a `mitpo-watchlist-refresh` workflow is already `Queued`, `Running`, or `WaitingApproval` — duplicate runs are not possible.

**State location.** `<runtime-root>/state/research/scheduler.json` — atomic writes, PID-unique tmp. Corrupt or missing state is treated as zero, never crashes the daemon.

**Diagnostics.** `pookie doctor` prints a "Research Scheduler" panel; the `/api/v1/console` JSON returned by the gateway includes a `scheduler` object that the web UI surfaces.

> **Migration note.** Earlier versions stored watchlists in the
> `research_watchlists` vault key. That value is now imported into
> `state/research/watchlists/` once on first startup; afterwards the vault
> field is read-only and `PUT /api/v1/settings/vault` will return HTTP 400
> if you try to write to it. Edit watchlists via:
>
>     pookie research watchlists apply --file watchlists.json
>
> or the Research panel in the web console.

## Run Sample Workflows

```powershell
Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:18800/api/v1/workflows `
  -ContentType "application/json" `
  -InFile .\examples\workflows\utm-validation.json
```

```powershell
Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:18800/api/v1/workflows `
  -ContentType "application/json" `
  -InFile .\examples\workflows\lead-route.json
```

```powershell
Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:18800/api/v1/workflows `
  -ContentType "application/json" `
  -InFile .\examples\workflows\sms-draft.json
```

## Use The Optional Brain

```powershell
$body = @{
  prompt = "Draft an SMS campaign for our April VIP launch and route it through the Mitto skill"
} | ConvertTo-Json

Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:18800/api/v1/brain/dispatch `
  -ContentType "application/json" `
  -Body $body
```

If no LLM provider is configured, the app still starts and the direct workflow forms still work.

## API Surface

- `GET /`
- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/console`
- `GET /api/v1/diagnostics`
- `GET /api/v1/status`
- `GET /api/v1/channels`
- `GET /api/v1/channels/status`
- `POST /api/v1/channels/whatsapp/test`
- `GET /api/v1/channels/whatsapp/webhook`
- `POST /api/v1/channels/whatsapp/webhook`
- `GET /api/v1/events`
- `POST /api/v1/messages`
- `GET /api/v1/messages/{id}`
- `GET /api/v1/workflows`
- `POST /api/v1/workflows`
- `POST /api/v1/workflows/plan`
- `GET /api/v1/approvals`
- `POST /api/v1/approvals/{id}/approve`
- `POST /api/v1/approvals/{id}/reject`
- `GET /api/v1/file-permissions`
- `POST /api/v1/file-permissions/{id}/approve`
- `POST /api/v1/file-permissions/{id}/reject`
- `GET /api/v1/skills`
- `POST /api/v1/skills/validate`
- `POST /api/v1/brain/dispatch`
- `GET /api/v1/settings/vault`
- `PUT /api/v1/settings/vault`

## Runtime Layout

PookiePaws uses `~/.pookiepaws/` by default.

- `workspace/` local file workspace
- `state/workflows/` workflow records
- `state/approvals/` approval records
- `state/filepermissions/` file permission records
- `state/runtime/status.json` latest runtime snapshot
- `state/audits/audit.jsonl` append-only audit stream
- `state/messages/` outbound channel message state and delivery updates
- `.security.json` host-side secrets and provider configuration

## Security Posture

- Workspace access is constrained under `~/.pookiepaws/workspace/`
- Existing symlink path segments are rejected in the sandbox
- File reads and writes can be wrapped in explicit operator approval
- Command execution is guarded by a read-only allowlist rather than a blacklist
- Secrets are read from host-side config and are not required for the console to run
- Adapter failures and file-access decisions are published back into the event stream

## Current Product Direction

PookiePaws is intentionally not trying to be a clone of a broader general-purpose agent product. The current direction is:

- local-first
- operator-controlled
- marketing-native
- approval-aware
- lightweight enough to audit and self-host easily

## Files To Start With

- [cmd/pookie/main.go](./cmd/pookie/main.go)
- [cmd/pookie/chat.go](./cmd/pookie/chat.go)
- [cmd/pookie/list.go](./cmd/pookie/list.go)
- [cmd/pookie/stack.go](./cmd/pookie/stack.go)
- [internal/cli/printer.go](./internal/cli/printer.go)
- [internal/cli/menu.go](./internal/cli/menu.go)
- [internal/gateway/server.go](./internal/gateway/server.go)
- [internal/gateway/ui/index.html](./internal/gateway/ui/index.html)
- [internal/gateway/ui/app.js](./internal/gateway/ui/app.js)
- [internal/security/permissioned_sandbox.go](./internal/security/permissioned_sandbox.go)

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [ROADMAP.md](./ROADMAP.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

Apache License 2.0. See [LICENSE](./LICENSE).
