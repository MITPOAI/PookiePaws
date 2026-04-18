# Changelog

All notable changes to PookiePaws should be documented in this file.

## [1.1.0] - Unreleased

### Added

- **Native Tool Calling**: `NativeOrchestrate` loop sends `[]ChatMessage + []ToolDefinition`
  to OpenAI-compatible APIs; processes `finish_reason: "tool_calls"` natively instead of
  text-JSON parsing
- **`JinaScraperTool` (`web_search`)**: replaces `WebSearchTool`; GETs `https://r.jina.ai/<url>`
  for clean Markdown output, capped at 8,000 chars
- **`ReadLocalFileTool` (`read_local_file`)**: reads workspace-sandboxed context files
  (brand guidelines, briefs), capped at 8,000 chars
- **`SecurityValidator`**: pre-execution middleware; path confinement for file tools +
  HITL `[⚠]` approval gate for `os_command`
- **`SystemPromptBase` constant**: marketing persona + 7 security rules injected as the
  first system message in every `NativeOrchestrate` conversation
- **`Definition() ToolDefinition`** on all four tools + `ToolRegistry.BuildDefinitions()`
- **`pookie version --check`**: forces a live GitHub Releases lookup, bypassing the
  on-disk cache, so operators can confirm the newest published build on demand
- **Cached update notifier**: interactive commands (`pookie`, `start`, `chat`, `init`,
  `list`, `doctor`) print a two-line stderr notice when a newer release is available;
  opt out with `POOKIEPAWS_NO_UPDATE_NOTIFIER=1` or `CI=1` (gh/npm convention)
- **`internal/updatecheck` package**: semver normalisation/comparison, GitHub Releases
  client with request timeouts and draft filtering, 24h file cache with atomic writes
  under `os.UserCacheDir()/pookiepaws/update-check.json` (overridable via
  `POOKIEPAWS_UPDATE_CACHE_PATH`), and an upgrade hint that prefers `winget`/`brew`
  when on `PATH` and falls back to the install scripts otherwise
- **`pookie research` subcommand**: `watchlists list|apply`, `refresh`, `schedule`,
  `status`, `recommendations` — full CLI surface for managing watchlists, the
  scheduler, and dossier recommendations from the terminal
- **Research scheduler**: daemon-only ticker (60s cadence) that submits
  `mitpo-watchlist-refresh` workflows according to the configured `research_schedule`
  (`manual|hourly|daily`); skips when a refresh is already in flight
- **`internal/scheduler` package**: pure decision function (manual/hourly/daily),
  atomic JSON state at `<runtime-root>/state/research/scheduler.json` with
  PID-unique tmp + corrupt-tolerant Load, ticker loop with in-flight suppression
- **Scheduler diagnostics**: `pookie doctor` prints scheduler state;
  `/api/v1/console` JSON includes a `scheduler` object when state exists

### Changed

- `Tool` interface gains `Definition() ToolDefinition` — all built-in tools updated
- `OrchestrateConfig` gains `Validator *SecurityValidator` (additive, nil-safe)
- `pookie chat` uses `NativeOrchestrate`; falls back to text-JSON `Orchestrate` for MCP
- `WebSearchTool` removed; `JinaScraperTool` registered under the same `web_search` name
- `pookie version` output now includes the cached upgrade hint when a newer release
  is known, in addition to the existing OS/arch and Go build info
- `dossier.Service` gained `GetWatchlist`, `DeleteWatchlist`, and `MaxLastRunAt`
  (used by the scheduler and CLI)
- `appStack` now constructs and exposes a shared `dossier.Service` instance
  (gateway endpoints continue to use their own instances; consolidation deferred)
- `pookie start` now launches the research scheduler goroutine alongside the HTTP server

### Security

- File path traversal validation enforced at orchestrator layer via `SecurityValidator`
  before any tool execution begins, in addition to the existing sandbox checks inside tools

## [1.0.0] - Unreleased

### Added

- **Pink ASCII art splash screen**: `pookie init` now displays a pink POOKIEPAWS banner
- **Quick-start 4+1 model menu**: init wizard offers DeepSeek V3.2, DeepSeek R1, Claude Opus 4.6, Meta Llama 4 70B, and Custom as a fast path before the full provider selection
- **REPL model info**: `pookie chat` now shows the connected model name and provider at startup
- **Unified MarketingChannel interface**: standardised Go interface (`Name`, `Kind`, `Status`, `Test`, `Execute`, `SecretKeys`) for all channel plugins, enabling the open-source community to extend the agent
- **InMemoryChannelRegistry**: thread-safe registry for marketing channel plugins
- **Resend email adapter**: full `net/http` integration with Resend API for outbound email (`send_email` operation)
- **HubSpot CRM adapter**: contact creation and update via HubSpot CRM v3 API (`create_contact`, `update_contact` operations)
- **Firecrawl/Jina research adapter**: web page to markdown conversion using Firecrawl API with Jina Reader fallback (`scrape` operation)
- **Slack webhook notifier**: sends workflow summaries to Slack channels via Block Kit format
- **Discord webhook notifier**: sends workflow summaries to Discord channels via embed format
- **Daily summary generator**: aggregates 24-hour workflow metrics for webhook delivery
- **Mock adapters**: MockResendAdapter, MockHubSpotAdapter, MockFirecrawlAdapter for testing

### Changed

- **Pookie Soft theme refined**: blush white backgrounds (#faf5f7), crisp white cards (#ffffff), deeper dark plum text (#2a1f30), warmer rose pink accents (#c4648a), lighter shadows
- **Existing adapters upgraded**: SalesmanagoAdapter, MittoAdapter, and WhatsAppAdapter now implement the unified MarketingChannel interface while retaining backward compatibility
- **CONTRIBUTING.md rewritten**: comprehensive guide for writing marketing channel plugins

## [0.6.0] - Unreleased

### Added

- **Raw-mode terminal REPL**: `pookie chat` now uses cross-platform raw terminal mode for proper backspace, Ctrl+C, and escape-sequence handling via the new `cli.ReadLine()` function — replaces the old `bufio.Scanner` which could not handle arrow keys or backspace
- **Intent router (casual chat)**: the brain service now classifies input as `casual_chat`, `run_workflow`, or `run_chain`; casual prompts ("Hello!", "What can you do?") return friendly conversational responses instead of crashing on non-JSON parse failures
- **Chained workflow execution**: new `run_chain` action allows the LLM to orchestrate multi-step pipelines where each step's output feeds into the next step's input automatically
- **`mitpo-researcher` skill**: fetch a public URL, strip HTML, and return a structured summary for competitor intelligence and marketing research
- **`mitpo-markdown-export` skill**: save text content as a timestamped Markdown file inside `workspace/exports/`
- **Research-to-export pipeline**: the brain can chain `mitpo-researcher` and `mitpo-markdown-export` in a single prompt to research a URL and export the results
- **OpenRouter model additions**: Cohere Command R3 (enterprise RAG) and Meta Llama 4 Instruct (open-weight) added to the OpenRouter provider presets

### Security

- Security policies added for `mitpo-researcher` (URL scheme and localhost validation) and `mitpo-markdown-export` (content-only allowed keys) in the skill execution interceptor

### Changed

- `Command.Validate()` now accepts `casual_chat` and `run_chain` actions in addition to `run_workflow`
- Brain system prompt updated to describe three valid response formats with clear routing guidance
- `ChainStep` type added to `Command` struct for multi-step pipeline definitions

## [0.5.2] - Unreleased

### Changed

- **2026 frontier model presets**: CLI wizard updated with GPT-5.4, Claude Opus 4.6 / Sonnet 4.6 (1M context), Gemini 3.1 Pro / Flash, DeepSeek V3.2 via OpenRouter
- **Conversation window expanded**: default turn limit raised from 8 to 24 to leverage 1M+ token context windows in multi-step marketing campaigns
- **Conversation window persistence**: window state saved to disk on every turn and restored on restart so context survives process restarts
- **Optimistic approval UI**: approve/reject actions update the DOM instantly with background server reconciliation and automatic rollback on failure
- **Canvas topology fix**: SVG link lines now use dynamic viewBox matching actual board dimensions; nodes and links share the same coordinate container
- **Audit view layout**: Action Approvals and File Permissions render as separate full-width rows instead of a 2-column grid
- **Prompt bar send button**: vertically centered using transform instead of fixed bottom offset

### Security

- **Prompt context boundaries**: system prompt sections now carry `[SYSTEM]`, `[OPERATOR]`, `[MEMORY]`, and `[TOOL-OUTPUT]` labels so the LLM can distinguish trusted instructions from untrusted data — prevents prompt injection from skill output or user content
- **Outbound channel policy layer**: formal `ChannelPolicy` rules per channel (WhatsApp: 1 recipient, SMS: 100 max, CRM: 1 lead) with `CheckChannelPolicy()` validator — enforces send limits independent of per-skill risk scoring

### Added

- **`pookie context` CLI command**: inspect the brain's current prompt size, memory narrative, extracted variables, registered skills, and the full rendered routing prompt (`--prompt` flag)
- **`pookie memory` CLI command**: inspect persistent brain memory entries and variables, prune all memory (`--prune`), or clear the conversation window only (`--prune-window`)

### Performance

- **Static asset caching**: `/ui/*` assets served with `Cache-Control: immutable` and `max-age=31536000`; cache-busted via `?v=` query param on each restart — eliminates redundant gzip compression
- **Targeted renders**: `render()` now only updates DOM panels for the active view instead of all 12 sub-renderers, reducing layout thrashing on state changes
- **Async memory compression**: `RecordWorkflow()` runs in a background goroutine so workflow completion responses are not blocked by LLM summarization
- **Audit log rotation**: audit.jsonl rotates at 5 MB, keeping up to 3 archived files — prevents unbounded disk growth
- **Exponential backoff reconnection**: WebSocket and SSE reconnect with exponential backoff (1s → 2s → 4s → ... → 30s max) instead of fixed 2s intervals, reducing thundering-herd load on server recovery

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
