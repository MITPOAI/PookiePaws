---
name: mitpo-dossier-generate
description: Generate a grounded competitor dossier, persist evidence and changes, and attach workflow-ready recommendations.
category: research
version: 1.0.0
tags: [research, dossier, evidence]
tools:
  - crawl_pages
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 2m
---
Generate a source-backed dossier for a competitor or topic. Persist the dossier, evidence records, detected changes, and recommendation queue into the local runtime so operators can review and act from one console.

## Inputs
- watchlist_id (string, optional): Existing watchlist identifier.
- name (string, optional): Watchlist display name.
- topic (string, optional): Topic or competitor label.
- company (string, optional): Brand or company the dossier is for.
- competitors (array, optional): Competitors to analyze.
- domains (array, optional): Domains to discover from.
- pages (array, optional): Explicit public pages to observe directly.
- focus_areas (array, optional): Pricing, positioning, offers, or other scoped lenses.
- market (string, optional): Market context.
- trusted_domains (string, optional): Comma or newline separated allowlisted domains for tracked pages.
- provider (string, optional): Research provider preference.
- debug (boolean, optional): Include debug-level research output.

## Outputs
- watchlist (object, required): Normalized persisted watchlist.
- dossier (object, required): Saved dossier metadata and synthesis.
- evidence (array, required): Persisted evidence records.
- changes (array, required): Added, modified, and removed evidence items.
- recommendations (array, required): Workflow-ready recommended actions.
