---
name: mitpo-ba-researcher
description: Scrape competitor sites and output a structured business analysis report.
category: research
version: 1.0.0
tags: [research, competitor, business-analysis]
tools:
  - crawl_pages
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 2m
---
Collect public competitor intelligence, structure findings into a business analysis report, and return actionable insights a marketer can use for positioning and campaign planning.

## Inputs
- company (string, required): Target company or competitor name.
- domains (array, optional): Public domains to research.
- focus_areas (array, optional): Specific areas to analyze (pricing, positioning, features).
- market (string, optional): Market or region context.

## Outputs
- company (string, required): The company the analysis covers.
- findings (array, required): Structured research findings.
- summary (string, required): Executive summary for the operator.
- sources (array, optional): Evidence sources supporting the findings.
