---
name: mitpo-watchlist-refresh
description: Refresh configured watchlists, persist dossiers and evidence, and update the recommendation queue.
category: research
version: 1.0.0
tags: [research, watchlist, autonomy]
tools:
  - crawl_pages
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 3m
---
Run the observe-extract-diff-summarize-recommend loop across configured watchlists. This is the control-plane entrypoint for recurring competitive research.

## Inputs
- watchlists_json (string, optional): JSON array of watchlists to refresh inline.
- watchlists (array, optional): Structured watchlists to refresh inline.
- trusted_domains (string, optional): Comma or newline separated allowlisted domains for tracked pages.

## Outputs
- watchlists (array, required): Watchlists that were processed.
- dossiers (array, required): Latest dossier records created in the run.
- changes (array, required): Detected change records.
- recommendations (array, required): Recommendation queue updates.
- warnings (array, optional): Partial failures or degraded coverage messages.
- watchlist_count (number, required): Count of refreshed watchlists.
- dossier_count (number, required): Count of dossiers generated.
- change_count (number, required): Count of change records created.
- recommendation_count (number, required): Count of recommendation records created.
