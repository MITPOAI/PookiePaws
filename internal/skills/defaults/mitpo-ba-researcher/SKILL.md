---
name: mitpo-ba-researcher
description: Run bounded live competitor research, synthesize structured findings, and output an operator-ready business analysis report.
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
Collect bounded public competitor intelligence, structure findings into a business analysis report, and return actionable insights a marketer can use for positioning and campaign planning.

## Inputs
- company (string, optional): Target brand or company name.
- competitors (array, optional): Competitors to research.
- domains (array, optional): Public domains to research or seed for degraded fallback.
- pages (array, optional): Explicit public pages to watch directly.
- focus_areas (array, optional): Specific areas to analyze (pricing, positioning, features).
- market (string, optional): Market or region context.
- country (string, optional): Search geo in ISO country-code form.
- location (string, optional): Search geo location string.
- max_sources (number, optional): Maximum kept sources, capped by bounded defaults.
- provider (string, optional): Research provider preference.
- debug (boolean, optional): Include raw markdown in the output for debugging.

## Outputs
- company (string, required): The company the analysis covers.
- findings (array, required): Structured research findings.
- summary (string, required): Executive summary for the operator.
- provider (string, optional): Provider that completed the run.
- competitor_notes (array, optional): Per-competitor synthesis notes.
- sources (array, optional): Evidence sources supporting the findings.
- warnings (array, optional): Coverage or scraping warnings.
- coverage (object, optional): Counts for discovered, scraped, kept, and skipped pages.
