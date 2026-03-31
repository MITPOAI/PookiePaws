# PookiePaws Roadmap

## Roadmap Principles

- Build the open-source foundation before shipping code
- Keep the core lightweight and auditable
- Add integrations through explicit boundaries
- Treat MITPO platform connectivity as a later layer, not a hidden dependency

## Phase 0: Foundation

Status: active

Goals:

- lock public positioning
- define architecture and contribution contracts
- establish repo governance and security rules
- prepare the project for implementation without overstating maturity

Deliverables:

- README, architecture, roadmap, security, changelog, contribution, and code of conduct docs
- Apache 2.0 licensing
- pull request governance that requires docs updates for structural changes

## Phase 1: Core Runtime

Goals:

- introduce the Go-based orchestration core
- define internal event flow and workflow execution boundaries
- add local configuration, workspace handling, and audit persistence

Success criteria:

- a minimal runnable daemon exists
- the runtime has a narrow, documented core
- destructive or unsafe operations are blocked by policy

## Phase 2: Local Web Workspace

Goals:

- introduce a browser-based operator workspace
- surface workflow state, audit events, and approvals
- document the operator experience and local deployment model

Success criteria:

- the UI can observe and control the local runtime
- high-risk steps can pause for human review
- the UI explains workflow state without exposing raw implementation noise

## Phase 3: Skills And External Integrations

Goals:

- add documented skill definitions
- introduce MCP-compatible integration boundaries
- ship initial reference workflows for research, strategy, and campaign operations

Success criteria:

- integrations are modular
- skill behavior and external execution remain separable
- the core does not become a hard-coded connector bundle

## Phase 4: MITPO Platform Connectivity

Goals:

- add documented MITPO-facing interfaces where appropriate
- connect the open-source foundation to the broader MITPO ecosystem
- preserve open-source core boundaries while layering platform-specific functionality

Success criteria:

- MITPO integration is explicit and documented
- private platform concerns do not leak into the open-source core contract
- roadmap and changelog clearly distinguish open-source features from MITPO-specific layers

## Ongoing Expectations

Across all phases:

- architecture changes must update the docs in the same pull request
- changelog entries must be added for structural changes
- public copy must remain accurate about what the repo ships today
