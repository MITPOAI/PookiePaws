# Contributing To PookiePaws

## Before You Start

PookiePaws is in a documentation foundation phase. Contributions should improve clarity, architecture quality, governance, and implementation readiness without pretending the runtime already exists.

Good contribution areas right now:

- documentation improvements
- architecture review and critique
- roadmap refinement
- governance and workflow proposals
- repository hygiene for future implementation

## Contribution Standard

Every contribution should make the repository easier to trust.

That means:

- keep claims accurate
- distinguish current behavior from planned behavior
- avoid marketing language that implies shipped functionality
- prefer explicit tradeoffs over vague optimism

## Documentation Governance

Documentation updates are required for any structural, behavioral, or public-facing change.

Unless clearly not applicable, the same pull request must review and update:

- [README.md](./README.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CHANGELOG.md](./CHANGELOG.md)

If one of those files does not need a change, explain why in the pull request.

## Scope Rules

During the foundation phase:

- do not add fake install commands
- do not claim production readiness
- do not introduce undocumented architecture changes
- do not commit secrets, customer data, or private MITPO assets
- do not imply MITPO API support before that work is documented and implemented

## How To Propose Work

For significant changes:

1. State the problem in concrete terms.
2. State the intended outcome.
3. Identify the docs affected.
4. Identify any security, workflow, or governance impact.

Small clarifications and typo fixes can go directly to pull requests.

## Pull Request Expectations

Each pull request should include:

- a concise summary of the change
- the docs updated as part of the change
- any assumptions or deferred work
- changelog updates for structural work

Use the pull request template in `.github/pull_request_template.md`.

## Review Criteria

Reviewers should look for:

- accuracy of project claims
- consistency with the architecture contract
- clean separation between current state and roadmap
- clear security and governance implications

## Community Conduct

By participating in this repository, you agree to follow [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Security

If your contribution touches trust boundaries, secrets, integrations, or execution policy, read [SECURITY.md](./SECURITY.md) before opening a pull request.
