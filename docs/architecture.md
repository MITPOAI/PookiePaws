# pookiepaws Ad Automation Architecture

This MVP adds a local-first social ad automation pipeline beside the existing PookiePaws operator runtime.

## Flow

1. `pookiepaws init` creates a local SQLite memory database under `POOKIEPAWS_HOME` or `~/.pookiepaws`.
2. `pookiepaws create-ad` loads the brand profile, builds a deterministic strategy and storyboard, generates mock assets, writes `edit_plan.json`, renders with FFmpeg, and saves project history.
3. `pookiepaws automate` expands a campaign request into a batch of content drafts, review reports, and a manual approval queue.
4. `pookiepaws studio campaign` creates a local marketing studio workspace with research, strategy, ad export files, browser workflow stubs, and content drafts.
5. `pookiepaws feedback add` records explicit user feedback and updates reusable prompt preferences. It does not train models or learn secretly.

## Modules

- `internal/memory`: SQLite brand profile, project history, search, export, reset, and feedback learning.
- `internal/planner`: deterministic ad strategy, storyboard, prompt generation, and edit-plan conversion.
- `internal/storyboard`: portable storyboard scene types.
- `internal/automation`: content batch request parsing, draft generation, and review queue output.
- `internal/studio`: local campaign workspace generator for research, strategy, ads, browser workflow stubs, and content automation.
- `internal/providers`: provider interface shared by mock, fal.ai, Runware, and future APIs.
- `internal/providers/mock`: no-cost image placeholder provider for local tests.
- `internal/renderer`: JSON edit-plan types and Go-to-Python render bridge.
- `scripts/media/render.py`: FFmpeg renderer for simple vertical motion-graphic ads.
- `internal/browser`: transparent dry-run scaffold for future Playwright workflows.
- `internal/capcut`: optional CapCut/Jianying adapter boundary.
- `internal/opencut`: optional OpenCut project bridge boundary.
- `internal/doctor`: local setup diagnostics for Go, Python, FFmpeg, provider env vars, memory write access, and renderer readiness.

## Safety Defaults

Browser workflows are dry-run first and the MVP does not post, bypass login, solve captchas, scrape private data, or hide automation. Content automation writes `requires_manual_approval` review queues and leaves posting status as `not_posted`. Remote providers require explicit API keys through environment variables.

## MVP Scope

The reliable path is local planning, mock generation, JSON edit plans, FFmpeg rendering, project history, and explicit feedback. fal.ai, Runware, Playwright execution, CapCut draft export, and OpenCut project bridging are scaffolded so they can be implemented behind stable interfaces after the local path is proven.

## Local Setup Checks

Run `pookiepaws doctor` or `pookiepaws setup check` before rendering. The command reports required Go/Python/FFmpeg readiness, optional provider API keys, the resolved memory path, and whether the local renderer can run.
