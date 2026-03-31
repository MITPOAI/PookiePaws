# PookiePaws

![PookiePaws banner](./assets/pookiepaws.svg)

[![Release](https://img.shields.io/github/v/release/mitpoai/pookiepaws?display_name=tag)](https://github.com/mitpoai/pookiepaws/releases)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/mitpoai/pookiepaws)](./LICENSE)

> Local-first marketing operations runtime from MITPO for research, strategy, approvals, and campaign execution.

PookiePaws is a pure-Go, stdlib-first marketing automation runtime built around one binary, a local operator console, approval-aware external actions, and a strict host-side secret boundary. It is intentionally narrower than a general-purpose AI workspace: the product is optimized for marketing operators who need clarity, auditability, and predictable workflow execution.

![Demo placeholder](./assets/pookiepaws.svg)

## Why PookiePaws

- It is local-first and inspectable instead of relying on a heavy hosted control plane.
- It keeps workflow execution visible through a compact operator console and event stream.
- It defaults to approval-gated outbound CRM and SMS actions.
- It supports local LLMs through an OpenAI-compatible boundary, so you can run a brain without a cloud API key.
- It keeps secrets host-side in `.security.json` and avoids putting provider credentials into prompts.
- It is designed as a marketing operations runtime, not a generic shell agent.

## What Makes It Different

- Small, single-binary runtime rather than a broad multi-service agent platform
- Operator-first console rather than a chat-only interface
- Workflow templates and direct forms for real marketing tasks
- Approval queue with human-readable action summaries
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
- Workspace sandboxing and host-side secret loading
- Runtime state and audit records stored under `~/.pookiepaws/`

## Quick Start

1. Build the runtime.

```powershell
go mod tidy
go build -o pookiepaws.exe cmd/pookiepaws/main.go
```

2. Copy the example security file and fill only the values you need.

```powershell
Copy-Item .security.example.json "$HOME\\.pookiepaws\\.security.json"
```

For local LLM use, this is enough:

```json
{
  "llm_base_url": "http://localhost:11434/v1/chat/completions",
  "llm_model": "gpt-oss:20b",
  "llm_api_key": ""
}
```

3. Start the app.

```powershell
.\pookiepaws.exe -addr 127.0.0.1:18800
```

Open [http://127.0.0.1:18800/](http://127.0.0.1:18800/).

## Run And Test

### Verify the build

```powershell
go test ./...
go build -o pookiepaws.exe cmd/pookiepaws/main.go
```

### Verify the console and status

```powershell
Invoke-RestMethod http://127.0.0.1:18800/api/v1/console
Invoke-RestMethod http://127.0.0.1:18800/api/v1/status
Invoke-RestMethod http://127.0.0.1:18800/api/v1/skills
```

### Run sample workflows

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

### Use the optional brain

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
- `GET /api/v1/approvals`
- `POST /api/v1/approvals/{id}/approve`
- `POST /api/v1/approvals/{id}/reject`
- `GET /api/v1/skills`
- `POST /api/v1/skills/validate`
- `POST /api/v1/brain/dispatch`

## Runtime Layout

PookiePaws uses `~/.pookiepaws/` by default.

- `workspace/` local file workspace
- `state/workflows/` workflow records
- `state/approvals/` approval records
- `state/runtime/status.json` latest runtime snapshot
- `state/audits/audit.jsonl` append-only audit stream
- `.security.json` host-side secrets and provider configuration

## Security Posture

- Workspace access is constrained under `~/.pookiepaws/workspace/`
- Existing symlink path segments are rejected in the sandbox
- Command execution is guarded by a read-only allowlist rather than a blacklist
- Secrets are read from host-side config and are not required for the console to run
- Adapter failures are published back into the event stream as `adapter.failed`

## Current Product Direction

PookiePaws is intentionally not trying to be a clone of a broader general-purpose agent product. The current direction is:

- local-first
- operator-controlled
- marketing-native
- approval-aware
- lightweight enough to audit and self-host easily

## Files To Start With

- [cmd/pookiepaws/main.go](./cmd/pookiepaws/main.go)
- [internal/gateway/server.go](./internal/gateway/server.go)
- [internal/gateway/ui/index.html](./internal/gateway/ui/index.html)
- [internal/brain/service.go](./internal/brain/service.go)
- [internal/security/sandbox.go](./internal/security/sandbox.go)

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [ROADMAP.md](./ROADMAP.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

Apache License 2.0. See [LICENSE](./LICENSE).
