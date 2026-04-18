---
name: mitpo-recommend-actions
description: Load workflow-ready dossier recommendations with source grounding and approval status.
category: research
version: 1.0.0
tags: [research, recommendations, workflow]
tools:
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 30s
---
Retrieve the current recommendation queue for a dossier or watchlist so the operator can queue, edit, or discard next actions from one place.

## Inputs
- dossier_id (string, optional): Dossier identifier to filter recommendations.
- watchlist_id (string, optional): Watchlist identifier to filter recommendations.

## Outputs
- dossier_id (string, optional): Dossier filter applied.
- watchlist_id (string, optional): Watchlist filter applied.
- recommendations (array, required): Source-backed recommended actions.
- recommendation_count (number, required): Number of recommendations returned.
