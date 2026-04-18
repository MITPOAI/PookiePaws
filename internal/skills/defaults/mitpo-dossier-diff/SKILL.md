---
name: mitpo-dossier-diff
description: Summarize the latest detected dossier changes for a watchlist.
category: research
version: 1.0.0
tags: [research, diff, dossier]
tools:
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 30s
---
Load the latest dossier delta for a watchlist and summarize what changed since the previous observation cycle.

## Inputs
- watchlist_id (string, optional): Watchlist identifier. When omitted, use the latest dossier on disk.

## Outputs
- watchlist_id (string, optional): The watchlist the diff belongs to.
- dossier_id (string, optional): Latest dossier identifier.
- summary (string, required): Human-readable change summary.
- changes (array, required): Structured change records.
