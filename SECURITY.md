# Security Policy

## Current Status

PookiePaws is currently in a documentation foundation phase. The repository does not yet ship a runnable agent, a local web UI, or production integrations.

That does **not** remove security responsibility. It means the current security focus is repository safety, disclosure discipline, and clear implementation requirements for future work.

## Reporting A Vulnerability

Do not open a public GitHub issue for:

- credential exposure
- private infrastructure details
- exploit paths that could be abused before remediation

Instead, report security issues privately to [support@mitpo.io](mailto:support@mitpo.io) with the subject line `PookiePaws Security Report`.

When possible, include:

- affected file or component
- reproduction steps
- impact assessment
- suggested mitigation if known

## Repository Safety Rules

Contributors must not commit:

- API keys, tokens, or secrets
- customer data or lead lists
- private MITPO credentials or internal configuration
- proprietary assets that are not approved for open-source release

If sensitive material is committed accidentally, treat it as a security incident and report it immediately.

## Planned Security Requirements

Future runtime work is expected to implement:

- workspace-restricted file access by default
- path validation and symlink protection
- explicit blocking of destructive execution paths
- secret handling outside model-visible context
- durable audit logging for sensitive actions
- human approval gates for high-risk workflow steps

These requirements are architectural obligations, not optional polishing work.

## Supported Versions

Until a runnable release exists, security review applies to the default branch and open pull requests.

## Disclosure Expectations

Security fixes should update the relevant docs and [CHANGELOG.md](./CHANGELOG.md) when changes are merged, while keeping exploit details private until maintainers decide disclosure is appropriate.
