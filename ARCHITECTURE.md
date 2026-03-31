# PookiePaws Architecture

## Purpose

This document defines the intended implementation shape of PookiePaws. It is an architecture contract for future contributors, not a statement that the runtime already exists.

## Current Repository State

The repository is in a documentation foundation phase.

Current state:

- No shipped runtime
- No local web UI
- No packaged integrations
- No public API surface

This document exists so implementation can start from a stable product boundary instead of redefining the project in every session.

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

The core engine should be implemented as a Go service with responsibility for:

- event intake and internal routing
- workflow orchestration
- subagent or task execution management
- policy enforcement before tool execution
- audit event emission

The core should remain small and defensible. Integration logic belongs at the edge, not in the orchestration core.

### 2. Local Web UI

The web UI should be a separate frontend application served alongside the local runtime or proxied by it.

Primary responsibilities:

- show active workflows and recent events
- expose skill and integration configuration
- surface audit details in plain language
- stop, resume, or approve sensitive workflow steps

The UI should optimize for operator clarity, not dashboard noise. It must explain what the system is doing, what it plans to do next, and where a human decision is required.

### 3. Skills And Integrations

The extensibility model should use two layers:

- documented skill definitions that describe repeatable marketing capabilities
- MCP-compatible integration services that perform external actions

This separation keeps behavioral intent distinct from external tool execution. Skills define workflow capability. Integrations define how the system touches outside tools.

### 4. Audit And Governance

Audit and governance are first-class parts of the design.

The intended system must support:

- persistent action history
- explicit tool-call visibility
- before-and-after records for destructive or externally visible changes
- human approval checkpoints for high-risk actions
- clear attribution of workflow, skill, and policy versions

### 5. Local State And Configuration

Implementation should keep local state explicit and inspectable.

Expected categories:

- workspace data
- local configuration
- audit history
- integration endpoints and non-secret identifiers

Secrets must not be exposed directly to model context. Secret injection and boundary enforcement are planned host responsibilities.

## Security Direction

Security constraints are part of the intended architecture even though the runtime is not yet implemented.

Planned controls:

- workspace-restricted file access by default
- symlink resolution and path validation
- explicit blocking of destructive shell operations
- secret handling outside model-visible prompt context
- approval gates for high-risk actions
- durable audit logging for sensitive workflow steps

These are implementation requirements, not optional enhancements.

## MITPO Integration Boundary

MITPO remains the parent brand and product ecosystem, but MITPO API connectivity is out of scope for the current phase.

Rules for the foundation:

- do not imply existing MITPO API support
- do not build docs around proprietary endpoints that are not public
- treat MITPO integration as future platform work layered on top of the open-source foundation

When MITPO integration begins, it should be added as documented interface work that preserves the open-source core boundary.

## Non-Goals For The Foundation Phase

The current phase should not introduce:

- speculative runtime code without matching documentation
- fake installation or deployment instructions
- claims of production readiness
- bundled private assets, credentials, or proprietary MITPO internals

## Documentation Contract

Future implementation work must keep the architecture contract current.

Any change that affects system shape, execution flow, integration boundaries, or governance rules must update:

- [README.md](./README.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CHANGELOG.md](./CHANGELOG.md)
