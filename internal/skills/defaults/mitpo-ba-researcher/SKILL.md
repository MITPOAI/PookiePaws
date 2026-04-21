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
Analyze the public competitor landscape for the target brand and named competitors. Check bounded public sources online, prioritize official domains, explicit pages, and trusted domains, capture source-backed claims on pricing, positioning, offers, features, promos, proof points, and CTA or messaging changes, and return an operator-ready business analysis report with actionable insights plus warnings when coverage is thin.

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
