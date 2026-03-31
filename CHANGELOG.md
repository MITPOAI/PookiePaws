# Changelog

All notable changes to PookiePaws should be documented in this file.

## [0.3.0] — Unreleased

### Added

- `cmd/pookie` — new single-binary CLI entrypoint that replaces `cmd/pookiepaws`
- `pookie start` — boots the local agent and web console (foreground, Ctrl+C to stop)
- `pookie status` — HTTP ping to a running agent; prints a formatted status box
- `pookie run <skill> [--input key=value ...]` — headless in-terminal skill execution with interactive approval handling
- `pookie install <owner/repo[@ref]>` — downloads a `SKILL.md` from a GitHub repository, validates against security sandbox rules, and saves to `workspace/skills/`
- `pookie init` — interactive setup wizard that collects LLM, CRM, and SMS credentials; writes `~/.pookiepaws/.security.json` atomically with mode 0600
- `pookie version` — prints the binary version string
- `internal/cli` package — lightweight terminal output using standard ANSI escape codes; no third-party dependencies
  - `Printer` with `Success`, `Error`, `Warning`, `Info`, `Accent`, `Dim`, `Rule`, `Banner`, `Box`
  - `Spinner` with `Start`, `Stop`, `UpdateLabel`
  - `ReadSecret` — cross-platform echo-suppressed password input (Unix: `stty -echo`; Windows: `kernel32.dll` Console API)
- `cmd/pookie/stack.go` — shared `buildStack` / `appStack` for both server and headless modes
- `cmd/pookie/roots.go` — `resolveRoots` with `--home` flag > `POOKIEPAWS_HOME` env > `os.UserHomeDir()` fallback

### Changed

- Primary binary renamed from `pookiepaws` to `pookie`; `cmd/pookiepaws` retained for backwards compatibility until the next major release

## [Unreleased — 0.2.x]

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
