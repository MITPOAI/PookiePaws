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

### 1. Core Engine

The current core engine is implemented as Go packages under `internal/engine` with responsibility for:

- event intake and internal routing
- workflow orchestration
- subagent or task execution management
- policy enforcement before tool execution
- audit event emission

Key implemented components:

- `EventBus` for typed, non-blocking event fan-out with drop accounting
- `SubTurnManager` for concurrent subagent lifecycle management
- `WorkflowCoordinator` for skill execution, approvals, adapter execution, and status snapshots

The core should remain small and defensible. Integration logic belongs at the edge, not in the orchestration core.

### 2. Local Web UI

The current gateway is served from the same binary via `internal/gateway`. It exposes REST plus SSE endpoints and embeds a compact operator console.

Primary responsibilities:

- show active workflows and recent events
- expose current skills, workflow templates, and direct workflow actions
- accept plain-language prompts for the LLM router when configured
- show provider status and approval queues clearly
- surface audit details in plain language
- stop, resume, or approve sensitive workflow steps

The UI should optimize for operator clarity, not dashboard noise. It must explain what the system is doing, what it plans to do next, and where a human decision is required.

### 3. Skills And Integrations

The extensibility model should use two layers:

- documented skill definitions that describe repeatable marketing capabilities
- MCP-compatible integration services that perform external actions

This separation keeps behavioral intent distinct from external tool execution. Skills define workflow capability. Integrations define how the system touches outside tools.

Current built-in skills:

- `utm-validator`
- `salesmanago-lead-router`
- `mitto-sms-drafter`

Current adapter posture:

- live HTTP adapters for SALESmanago and Mitto
- approval-gated execution for all outward-facing actions
- no MITPO-private integration in the open-source core

### 4. LLM Brain

The current LLM bridge is implemented as a narrow dispatch service:

- it calls an OpenAI-compatible completion endpoint over `net/http`
- it instructs the model to emit strict JSON only
- it parses that JSON into a workflow command
- it validates the selected skill before submission
- it publishes brain command events before routing into the normal workflow path

This keeps model output constrained and reduces the amount of free-form reasoning that can leak into tool execution.

The intended provider strategy is still narrow:

- one OpenAI-compatible provider boundary first
- local LLM support as a first-class path
- direct Claude, Gemini, and OpenRouter adapters deferred until the provider boundary and redaction model are more mature

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
