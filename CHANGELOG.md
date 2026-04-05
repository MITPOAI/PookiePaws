# Changelog

All notable changes to PookiePaws should be documented in this file.

## [0.5.1] - Unreleased

### Added

- **Event bus context propagation**: `EventBus.Publish` now accepts `context.Context` so in-flight event delivery can be cancelled during graceful shutdown, saving resources on `Ctrl+C`
- **Gzip compression middleware**: all HTTP responses (except SSE streams and WebSocket upgrades) are gzip-compressed, reducing dashboard payload sizes
- **`--verbose` CLI flag**: `pookie start --verbose` prints millisecond-level request timing logs for debugging performance bottlenecks
- **Skeleton loaders**: dashboard panels show shimmering placeholder cards while the initial API response loads, eliminating blank-screen flash
- **Sidebar section labels**: navigation grouped under "Overview", "Operations", and "Governance" headings for visual hierarchy
- **Sidebar footer**: version indicator and tagline anchored to the bottom of the sidebar

## [0.5.0] - Unreleased

### Added

- **Interactive `pookie init` wizard overhaul**:
  - new Pookie splash, ANSI palette, and arrow-key-first setup flow
  - curated 2026-compatible brain presets for OpenAI, Anthropic, Google, OpenRouter, Ollama, and LM Studio / Local
  - second-step model menus that persist exact model IDs and full chat-completions URLs
  - wizard-native masked secret input with no Unix `stty` shell-out
  - provider connectivity checks with retry, re-enter, and skip handling
  - checkbox-driven marketing channel activation for Meta WhatsApp, Mitto SMS, and SALESmanago
  - final review screen with redacted secrets and destination path before save
  - atomic config persistence helper that writes `~/.pookiepaws/.security.json` with strict `0600` permissions on Unix and best-effort Windows handling
- **Smart Sandbox**: risk-based auto-approval policy - low-risk actions can execute silently while high-risk sends pause for operator approval
  - `AutoApprovalPolicy` type with `Enabled` and `MaxRisk` fields
  - `SetAutoApprovalPolicy` / `GetAutoApprovalPolicy` on `StandardWorkflowCoordinator`
  - `GET/PUT /api/v1/settings/auto-approval` endpoint
  - UI toggle in the Settings view ("Auto-approve low-risk actions")
  - New event type `approval.auto_approved` in the audit trail
- **WhatsApp incoming message routing**: webhook now parses inbound messages from `entry[*].changes[*].value.messages[*]` and routes text to the brain service for AI-driven workflow dispatch
  - `ParseIncomingMessages()` on `WhatsAppAdapter` and `ChannelAdapter` interface
  - `ChannelIncomingMessage` type in engine
  - `EventChannelIncoming` event type for audit trail visibility
  - Deduplication by message ID to prevent re-processing on Meta retries
- Three new marketing skills:
  - `mitpo-ba-researcher` - business analysis and competitor research from public sources (low risk, report-only)
  - `mitpo-creative-director` - brand voice copy generation with tone analysis (low risk, report-only)
  - `mitpo-seo-auditor` - URL keyword density and technical SEO audit (low risk, report-only)
- Security policies for the three new skills in `SkillExecutionInterceptor` (all `risk: "low"`)

### Changed

- **Full UI/UX rewrite**: minimalist design system with Stripe/Vercel aesthetic
  - Flat surfaces, 1px borders, generous whitespace, restrained color palette
  - System font stack (`system-ui, -apple-system, 'Segoe UI', sans-serif`)
  - Audit rail rendered as a dark monospace terminal panel
  - Three themes retained (light, dark, soft) with refined token values
  - All existing `app.js` DOM bindings preserved
- Version bumped to 0.5.0

## [0.4.0] - Unreleased

### Added

- Interactive arrow-key menu when `pookie` is invoked with no arguments - pure ANSI, zero dependencies
- `pookie chat` - terminal AI REPL for conversing with Pookie in plain English; routes prompts to the brain service and displays workflow results inline
- `pookie list` - tabular listing of all installed marketing skills (built-in and workspace)
- `pookie --version` / `pookie -v` now prints OS, architecture, and Go version alongside the release string
- `internal/cli.RunMenu` - cross-platform interactive menu using raw terminal mode (Unix `tcsetattr`, Windows `kernel32.dll`)
- `internal/cli.Printer.IsColor` accessor for external colour-aware formatting
- Chat REPL slash commands: `/skills`, `/clear`, `/exit`, `/help`

### Changed

- `pookie --help` / `pookie -h` now shows `chat` and `list` in the command table
- Graceful shutdown in `pookie start` now explicitly closes the engine stack and prints a farewell message
- Version bumped to 0.4.0

## [0.3.0] - Unreleased

### Added

- `cmd/pookie` - new single-binary CLI entrypoint that replaces `cmd/pookiepaws`
- `pookie start` - boots the local agent and web console (foreground, Ctrl+C to stop)
- `pookie status` - HTTP ping to a running agent; prints a formatted status box
- `pookie run <skill> [--input key=value ...]` - headless in-terminal skill execution with interactive approval handling
- `pookie install <owner/repo[@ref]>` - downloads a `SKILL.md` from a GitHub repository, validates against security sandbox rules, and saves to `workspace/skills/`
- `pookie init` - interactive setup wizard that collects LLM, CRM, and SMS credentials; writes `~/.pookiepaws/.security.json` atomically with mode 0600
- `pookie version` - prints the binary version string
- `internal/cli` package - lightweight terminal output using standard ANSI escape codes; no third-party dependencies
  - `Printer` with `Success`, `Error`, `Warning`, `Info`, `Accent`, `Dim`, `Rule`, `Banner`, `Box`
  - `Spinner` with `Start`, `Stop`, `UpdateLabel`
  - `ReadSecret` - cross-platform echo-suppressed password input
- `cmd/pookie/stack.go` - shared `buildStack` / `appStack` for both server and headless modes
- `cmd/pookie/roots.go` - `resolveRoots` with `--home` flag > `POOKIEPAWS_HOME` env > `os.UserHomeDir()` fallback

### Changed

- Primary binary renamed from `pookiepaws` to `pookie`; `cmd/pookiepaws` retained for backwards compatibility until the next major release

## [Unreleased - 0.2.x]

### Added

- Go module and `cmd/pookiepaws` command entrypoint
- Core engine package with `EventBus`, `SubTurnManager`, and `WorkflowCoordinator`
- Local gateway with embedded operator console and REST plus SSE endpoints
- Brain package with structured LLM command parsing and workflow dispatch
- File-backed runtime state with JSON records and JSONL audit logs
- Workspace sandbox, destructive-command guard, and host-side `.security.json` secret handling
- MCP package with stdio and Streamable HTTP transport implementations
- Built-in default skills for UTM validation, Salesmanago lead routing, and Mitto SMS drafting
- Live SALESmanago and Mitto HTTP adapters for approval-aware action execution
- Repository `.gitignore` for runtime state, logs, builds, and research snapshots
- README quick start, runtime examples, and LLM/adapter configuration guidance
- Example `.security.example.json` and sample workflow payloads for open-source onboarding
