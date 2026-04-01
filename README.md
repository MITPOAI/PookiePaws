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
- Optional brain routing from free-text to structured workflow commands
- Live HTTP adapters for SALESmanago and Mitto
- Approval-gated workspace reads and writes through `PermissionedSandbox`
- Runtime state and audit records stored under `~/.pookiepaws/`

## Install

### One-liner (recommended)

**Windows** — paste into PowerShell:
```powershell
powershell -c "irm https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.ps1 | iex"
```

**macOS / Linux** — paste into Terminal:
```bash
curl -fsSL https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.sh | bash
```

The installer automatically detects your OS and architecture, downloads the right binary from the [latest release](https://github.com/MITPOAI/PookiePaws/releases/latest), and adds `pookie` to your PATH. No Go toolchain required.

### Manual download

Download the binary for your platform from the [Releases page](https://github.com/MITPOAI/PookiePaws/releases/latest):

| Platform | File |
|----------|------|
| Windows 64-bit | `pookie_*_windows_amd64.zip` |
| macOS Apple Silicon | `pookie_*_darwin_arm64.tar.gz` |
| macOS Intel | `pookie_*_darwin_amd64.tar.gz` |
| Linux 64-bit | `pookie_*_linux_amd64.tar.gz` |
| Linux ARM | `pookie_*_linux_arm64.tar.gz` |

## Quick Start

1. Ensure you are on a working Go 1.22 toolchain.

```powershell
go env -w GOTOOLCHAIN=go1.22.12+auto
go list std > $null
```

2. Synchronize modules and build the binary.

```powershell
go mod tidy
go build -o pookie.exe ./cmd/pookie
```

3. Run the interactive setup wizard.

```powershell
.\pookie.exe init
```

The wizard collects your LLM provider URL, model name, and API key. For a
local Ollama setup this is enough (leave the API key blank):

```
LLM base URL  >  http://localhost:11434/v1/chat/completions
LLM model     >  llama3.2:latest
LLM API key   >  [blank]
```

Credentials are written to `~\.pookiepaws\.security.json` with mode 0600.

4. Start the agent.

```powershell
.\pookie.exe start
```

Open [http://127.0.0.1:18800/](http://127.0.0.1:18800/).

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
Invoke-RestMethod http://127.0.0.1:18800/api/v1/skills
```

## CLI Commands

```
pookie start              Boot the agent (foreground; Ctrl+C to stop)
pookie status             Check whether the agent is running
pookie run <skill>        Execute a skill directly in this terminal
pookie install <repo>     Install a skill from GitHub
pookie init               First-run setup wizard
pookie version            Print version
```

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
- `GET /api/v1/console`
- `GET /api/v1/status`
- `GET /api/v1/events`
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
- [cmd/pookie/stack.go](./cmd/pookie/stack.go)
- [internal/cli/printer.go](./internal/cli/printer.go)
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
