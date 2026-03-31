# PookiePaws

> Open-source AI marketing automation foundation from MITPO for research, strategy, and campaign workflows.

PookiePaws is MITPO's open-source foundation for a lightweight marketing automation agent. The target shape is a hybrid system: a local-first agent runtime with a browser-based workspace for planning, workflow execution, audit visibility, and safe human approval on high-risk actions.

## Status

**Foundation phase.** This repository currently defines the product direction, architecture contract, contribution rules, and community policies.

What exists today:

- Open-source positioning and documentation
- Architecture and roadmap contracts
- Contribution, security, and governance rules

What does not exist yet:

- A runnable daemon or local web UI
- MITPO API integration
- Production integrations, skills, or deployment packages

MITPO API support is planned for a later phase. It is roadmap work, not current behavior.

## Why PookiePaws

Marketing teams need more than content generation. They need a system that can move from research into strategy, then from strategy into structured execution, without losing context or removing operator control.

PookiePaws is intended to provide that foundation:

- Research and strategy workflows in one system
- Safe automation for repeatable marketing operations
- A lightweight runtime that does not require a heavy SaaS stack
- A clear boundary between agent reasoning, tool execution, and human approval

## Relationship To MITPO

MITPO is the parent product and brand at [mitpo.io](https://www.mitpo.io/). PookiePaws is not the MITPO web app. It is the open-source agent layer MITPO intends to build in public, with MITPO-specific API connectivity and product integrations added later as separate implementation work.

## Planned System Shape

The implementation contract for future work is:

- **Core engine:** a lightweight Go daemon responsible for agent orchestration, event handling, workflow execution, and policy enforcement
- **Local web UI:** a browser-based workspace for monitoring workflows, configuring skills, reviewing audit events, and approving sensitive actions
- **Skills and integrations:** modular capabilities exposed through MCP-compatible services and documented skill definitions
- **Audit and governance:** explicit action logging, workspace restrictions, approval checkpoints, and operational guardrails

This repository should evolve around that shape unless a future architecture decision explicitly changes it in [ARCHITECTURE.md](./ARCHITECTURE.md).

## Foundation Phase Priorities

The current phase is docs-first by design. The immediate goal is to make the project legible to contributors before any runtime code appears.

Current priorities:

1. Lock the public positioning of the project
2. Define the intended architecture and phase boundaries
3. Establish contribution, security, and documentation governance rules
4. Prepare the repo for implementation work without overstating current capabilities

## Documentation Map

- [ARCHITECTURE.md](./ARCHITECTURE.md) defines the implementation shape and system boundaries
- [ROADMAP.md](./ROADMAP.md) defines delivery phases
- [CONTRIBUTING.md](./CONTRIBUTING.md) defines contribution and documentation rules
- [SECURITY.md](./SECURITY.md) defines security reporting and repository safety expectations
- [CHANGELOG.md](./CHANGELOG.md) tracks structural changes to the project definition

## Contribution Rule

Any structural, behavioral, or public-facing change must update the relevant documentation in the same pull request. At minimum, contributors should review:

- [README.md](./README.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CHANGELOG.md](./CHANGELOG.md)

If a change does not require one of those files, the PR should state why.

## Contributing

Start with [CONTRIBUTING.md](./CONTRIBUTING.md). The current repo is best suited for:

- documentation improvements
- architecture review
- roadmap refinement
- governance and workflow proposals

Implementation work should follow the documented architecture contract and keep the docs in sync.

## License

PookiePaws is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
