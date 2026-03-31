# Changelog

All notable changes to PookiePaws should be documented in this file.

## [Unreleased]

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
