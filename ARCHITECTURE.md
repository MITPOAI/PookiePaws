# PookiePaws Architecture

## Purpose

This document defines the intended implementation shape of PookiePaws. It is the architecture contract for future contributors and now also describes the first runnable runtime that exists in this repository.

## Current Repository State

The repository now contains a first runnable Go implementation.

Current state:

- Go module and command entrypoint
- In-process event bus and subturn manager
- Workflow coordinator with approval-aware adapter execution
- Local gateway using `net/http` plus SSE
- Operator console with direct workflow forms, approvals, templates, and provider status
- Structured LLM dispatch path using an OpenAI-compatible HTTP client
- JSON and JSONL runtime state
- Embedded default skill manifests and built-in skill implementations
- Live SALESmanago and Mitto HTTP adapters
- Approval-gated WhatsApp outbound adapter plus delivery-status ingestion

This document exists so implementation can continue from a stable product boundary instead of redefining the project in every session.

## Product Boundary

PookiePaws is MITPO's open-source marketing automation foundation.

It is intended to cover:

- research-to-strategy workflow orchestration
- repeatable campaign and operations workflows
- modular integrations through MCP-compatible boundaries
- operator visibility, auditability, and approval control

It is not intended to be:

- a copy of the MITPO web application
- a generic content spam engine
- a hidden wrapper around private MITPO APIs in the foundation phase

## Target Runtime Model

The target model is a hybrid local-first system:

1. A lightweight Go daemon runs locally on a workstation, managed host, or small server.
2. That daemon exposes a browser-based workspace over localhost or another explicitly configured local address.
3. The workspace presents agent status, workflow execution, audit events, approvals, and configuration.
4. External systems are connected through MCP-style integration boundaries rather than being hard-wired into the core.

This keeps the core narrow, the UI accessible, and the integration surface extensible.

## Planned System Components

### 0. CLI Entry Point (`cmd/pookie`)

The `pookie` binary is the single distributable artefact. It implements a
lightweight stdlib-based command router (no third-party CLI framework) and
dispatches to five commands:

| Command | Description |
|---|---|
| `start` | Initialises the full engine stack, starts the HTTP server, blocks until `Ctrl+C` |
| `chat` | Terminal AI REPL — routes plain-English prompts through the brain service using raw terminal mode (`cli.ReadLine`) for proper backspace, Ctrl+C, and escape handling |
| `list` | Tabular listing of all installed marketing skills (built-in and workspace) |
| `status` | HTTP-pings `/api/v1/status` on a running agent; prints a formatted summary box |
| `run <skill>` | Boots the engine headlessly, submits a workflow, polls for completion, handles approval gates interactively |
| `install <owner/repo>` | Downloads and validates a remote `SKILL.md`; saves to `workspace/skills/` |
| `init` | Interactive wizard that writes `~/.pookiepaws/.security.json` (mode 0600) |

When invoked with no arguments, `pookie` presents an interactive arrow-key
selection menu instead of printing help text. The menu is implemented using
raw ANSI escape sequences and cross-platform raw terminal mode (Unix
`tcsetattr` via `SYS_IOCTL`, Windows `kernel32.dll` console APIs). No
third-party TUI libraries are used.

Key design decisions:

- **Smart Sandbox auto-approval**: The `StandardWorkflowCoordinator` stores
  an `AutoApprovalPolicy` via `atomic.Value`. When enabled, actions whose
  interceptor risk level is at or below `MaxRisk` have `RequiresApproval`
  overridden to `false` before the approval record is created. This keeps
  skill code unchanged — skills still declare intent, and the coordinator
  applies policy at runtime.
- `cmd/pookie/stack.go` extracts `buildStack` so `start` and `run` share
  identical initialisation without duplication.
- `cmd/pookie/roots.go` centralises OS-agnostic path resolution:
  `--home` flag → `POOKIEPAWS_HOME` env → `os.UserHomeDir()` + `filepath.Join`.
- `internal/cli` provides all terminal styling (ANSI colours, spinner, boxed
  panels, interactive menus) with zero external dependencies. Colour is
  suppressed when `NO_COLOR` is set or `TERM=dumb`. Raw terminal mode for
  the interactive menu uses platform-specific files (`rawmode_unix.go`,
  `rawmode_windows.go`) following the same `kernel32.dll`/`syscall` pattern
  used by `ReadSecret`. The `cli.ReadLine()` function exposes this raw mode
  for the `chat` REPL, providing proper backspace, escape-sequence handling,
  and line repainting. Falls back to buffered `ReadString` when stdin is piped.

### 1. Core Engine

The current core engine is implemented as Go packages under `internal/engine` with responsibility for:

- event intake and internal routing
- workflow orchestration
- subagent or task execution management
- policy enforcement before tool execution
- audit event emission

Key implemented components:

- `EventBus` for typed, non-blocking event fan-out with drop accounting
  and `context.Context` propagation for cancellation during graceful shutdown
- `SubTurnManager` for concurrent subagent lifecycle management
- `WorkflowCoordinator` for skill execution, approvals, adapter execution, and status snapshots

The core should remain small and defensible. Integration logic belongs at the edge, not in the orchestration core.

### 2. Local Web UI

The current gateway is served from the same binary via `internal/gateway`. It
exposes REST plus SSE endpoints and embeds a compact operator console. All
non-streaming HTTP responses are gzip-compressed via stdlib middleware. The
`--verbose` CLI flag enables request-level timing logs for performance
debugging.

Primary responsibilities:

- show active workflows and recent events
- expose current skills, workflow templates, and direct workflow actions
- accept plain-language prompts for the LLM router when configured
- show provider status and approval queues clearly
- surface audit details in plain language
- stop, resume, or approve sensitive workflow steps

The UI should optimize for operator clarity, not dashboard noise. It must explain what the system is doing, what it plans to do next, and where a human decision is required.

Approval and file-permission actions use optimistic UI: the DOM updates
instantly on user click, with background server reconciliation and automatic
rollback on failure.

Performance optimizations:

- Static assets served with `Cache-Control: immutable` headers, cache-busted
  via `?v=` query param — eliminates redundant gzip compression between page
  loads
- `render()` uses targeted rendering: only the active view's panels are
  updated on state changes, reducing DOM writes by ~70% on average
- SSE and WebSocket reconnection uses exponential backoff (1s–30s) to avoid
  thundering-herd load during server restarts

### 3. Skills And Integrations

The extensibility model should use two layers:

- documented skill definitions that describe repeatable marketing capabilities
- MCP-compatible integration services that perform external actions

This separation keeps behavioral intent distinct from external tool execution. Skills define workflow capability. Integrations define how the system touches outside tools.

Current built-in skills:

- `utm-validator`
- `salesmanago-lead-router`
- `mitto-sms-drafter`
- `whatsapp-message-drafter`
- `mitpo-ba-researcher` — business analysis and competitor intelligence
- `mitpo-creative-director` — brand voice copy generation
- `mitpo-seo-auditor` — URL keyword density and technical SEO audit
- `mitpo-researcher` — fetch a public URL, strip HTML, and summarize content for marketing intelligence
- `mitpo-markdown-export` — save text content as a timestamped Markdown file to `workspace/exports/`

Skills can be executed in sequence via the `run_chain` action, where the output
of each step is merged into the input of the next. For example, the brain can
chain `mitpo-researcher` and `mitpo-markdown-export` to research a competitor's
pricing page and export the summary in a single prompt.

Current adapter posture:

- live HTTP adapters for SALESmanago and Mitto
- outbound WhatsApp channel adapter behind a provider-facing abstraction
- inbound WhatsApp message parsing (`ParseIncomingMessages`) with brain dispatch
  for AI-driven workflow routing from incoming customer messages
- approval-gated execution for all outward-facing actions
- risk-based auto-approval for low-risk skills via Smart Sandbox policy
- no MITPO-private integration in the open-source core

#### Channel Registry

All marketing channel plugins now implement the unified `MarketingChannel`
interface defined in `internal/engine/types.go`:

```
Name() string
Kind() string  // "crm", "sms", "email", "whatsapp", "research"
Status(secrets) ChannelProviderStatus
Test(ctx, secrets) (ChannelProviderStatus, error)
Execute(ctx, action, secrets) (AdapterResult, error)
SecretKeys() []string
```

The `InMemoryChannelRegistry` in `internal/adapters/registry.go` stores
registered channels and supports lookup by name or kind. Community developers
extend PookiePaws by implementing this single interface.

Built-in channels:
- WhatsApp (Meta Cloud API v23) - outbound messaging and delivery tracking
- Mitto SMS - single and bulk SMS delivery
- SALESmanago CRM - lead routing and upsert
- Resend - transactional and marketing email
- HubSpot - CRM contact creation and updates
- Firecrawl/Jina - web page to markdown research

Webhook notifiers (Slack and Discord) implement the `WebhookNotifier`
interface for delivering daily workflow summaries to team channels.

### 4. LLM Brain

The current LLM bridge is implemented as a narrow dispatch service:

- it calls an OpenAI-compatible completion endpoint over `net/http`
- it instructs the model to emit strict JSON only
- it parses that JSON into one of three command actions:
  - `run_workflow` — a single marketing workflow
  - `casual_chat` — conversational response for greetings and questions
  - `run_chain` — multi-step pipeline where each step's output feeds into the next
- it validates the selected skill(s) before submission
- it publishes brain command events before routing into the normal workflow path

The intent router eliminates the need for a separate classification step — the
system prompt instructs the LLM to classify input and select the appropriate
action format. Casual input that previously crashed the parser is now handled
gracefully as a `casual_chat` response.

The provider strategy supports six provider families through two protocol boundaries:

- **OpenAI-compatible**: direct endpoint for OpenAI (GPT-5.4, o3), Anthropic (Claude Opus 4.6, Sonnet 4.6), Google (Gemini 3.1 Pro/Flash), OpenRouter (DeepSeek V3.2, R1, Qwen 3.5, Cohere Command R3, Meta Llama 4 Instruct), Ollama, and LM Studio
- **MCP bridge**: stdio and Streamable HTTP transports for MCP-compatible model servers

All model selection is externalized through secrets (`llm_model`, `llm_base_url`). The CLI `init` wizard provides curated presets but any OpenAI-compatible endpoint works. The conversation window defaults to 24 turns to leverage 1M+ token context windows available in 2026 frontier models.

#### Native Tool Calling (v1.1.0)

PookiePaws v1.1.0 adds native OpenAI-compatible tool calling alongside the existing
text-JSON ReAct loop:

- **`NativeClient` interface** — `CompleteNative(ctx, []ChatMessage, []ToolDefinition)` on
  `OpenAICompatibleClient`. MCP providers do not implement this; the orchestrator detects
  and falls back automatically to the text-JSON `Orchestrate` path.
- **`NativeOrchestrate` loop** — manages a stateful `[]ChatMessage` array. Sends all tool
  schemas as JSON Schema definitions. Dispatches on `finish_reason`: `"tool_calls"` → route
  through `SecurityValidator`, execute tool, append `role:"tool"` result message, loop;
  `"stop"` → parse content as final response.
- **`SecurityValidator`** — pre-execution middleware in `package brain`. Validates file paths
  via `WorkspaceSandbox.ResolveWithinWorkspace` for `export_markdown` and `read_local_file`,
  returning `{"error":"Security Violation: Path out of bounds."}` to the model when blocked.
  For `os_command`, triggers the HITL prompt `[⚠] Pookie wants to run: <cmd>. Allow? [Y/n]`
  via `ApprovalFunc` before execution. After the validator approves an `os_command`, the tool
  is invoked with a pass-through `Approve` to prevent a second prompt.
- **Tool set** — `web_search` (JinaScraperTool via `r.jina.ai`), `export_markdown`
  (workspace-sandboxed write), `read_local_file` (workspace-sandboxed read),
  `os_command` (ExecGuard-allowlisted + HITL).
- **`Definition() ToolDefinition`** — all tools implement this method returning a JSON Schema
  function definition. `ToolRegistry.BuildDefinitions()` collects them for the native API
  request. The text-JSON `Orchestrate` path is unchanged and continues to use `ParameterSchema()`.

### 5. Audit And Governance

Audit and governance are first-class parts of the design.

The intended system must support:

- persistent action history
- explicit tool-call visibility
- before-and-after records for destructive or externally visible changes
- human approval checkpoints for high-risk actions
- clear attribution of workflow, skill, and policy versions

### 6. Local State And Configuration

Implementation should keep local state explicit and inspectable.

Expected categories:

- workspace data
- local configuration
- audit history
- integration endpoints and non-secret identifiers

Secrets must not be exposed directly to model context. Secret injection and boundary enforcement are planned host responsibilities.

## Security Direction

Security constraints are part of the implemented runtime and remain part of the long-term architecture contract.

Planned controls:

- workspace-restricted file access by default
- symlink resolution and path validation
- explicit blocking of destructive shell operations
- secret handling outside model-visible prompt context
- approval gates for high-risk actions
- durable audit logging for sensitive workflow steps

Implemented controls in the current runtime:

- runtime root at `~/.pookiepaws/`
- workspace root at `~/.pookiepaws/workspace/`
- `filepath.EvalSymlinks` enforcement through the workspace sandbox
- `ExecGuard` blocking destructive patterns such as `rm -rf`, `mkfs`, `diskpart`, `shutdown`, and `reboot`
- `.security.json` secret storage at the host boundary
- append-only JSONL audit events
- provider health/readiness endpoints and exportable diagnostics

## MITPO Integration Boundary

MITPO remains the parent brand and product ecosystem, but MITPO API connectivity is out of scope for the current phase.

Rules for the foundation:

- do not imply existing MITPO API support
- do not build docs around proprietary endpoints that are not public
- treat MITPO integration as future platform work layered on top of the open-source foundation

When MITPO integration begins, it should be added as documented interface work that preserves the open-source core boundary.

## Non-Goals For The Foundation Phase

The current phase should not introduce:

- claims of production readiness
- bundled private assets, credentials, or proprietary MITPO internals
- direct code fusion from OpenClaw or NemoClaw
- live vendor coupling that would force private APIs or heavy dependencies

## Documentation Contract

Future implementation work must keep the architecture contract current.

Any change that affects system shape, execution flow, integration boundaries, or governance rules must update:

- [README.md](./README.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CHANGELOG.md](./CHANGELOG.md)
