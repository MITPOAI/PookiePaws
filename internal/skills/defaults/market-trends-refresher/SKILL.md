---
name: market-trends-refresher
description: Refresh live market signals for brand, competitor, and channel planning.
category: research
version: 1.0.0
tags: [market-intelligence, trends, research]
tools:
  - fetch_sources
  - cluster_signals
events:
  - workflow.submitted
transport: jsonrpc
rpc_method: marketing.trends.refresh
rpc_notifications:
  - marketing.skill.progress
approval_policy: report_only
timeout: 3m
---
Refresh recent market signals, cluster them into themes marketers can act on, and return a concise evidence-backed trend summary.

## Inputs
- brand (string, required): Brand being monitored.
- regions (array, optional): Markets to watch for movement.
- channels (array, optional): Marketing channels to track.
- topics (array, optional): Themes or keyword clusters to prioritize.
- competitors (array, optional): Competitor brands to compare against.
- lookback_days (number, required): Number of days to include in the scan.
- max_sources (number, required): Maximum sources to collect before summarizing.
- include_sentiment (boolean, optional): Include positive or negative sentiment signals.

## Outputs
- summary (string, required): Short operator-ready summary of the trend landscape.
- signals (array, required): Ranked trend signals with direction and confidence.
- recommended_actions (array, optional): Suggested next actions for the marketer.
- sources (array, optional): Evidence sources supporting the findings.

## RPC Notifications
- marketing.skill.progress
