---
name: seo-audit-runner
description: Run a focused SEO audit and translate crawl issues into marketer language.
category: seo
version: 1.0.0
tags: [seo, audit, search]
tools:
  - crawl_site
  - evaluate_metadata
events:
  - workflow.submitted
transport: jsonrpc
rpc_method: marketing.seo.audit
rpc_notifications:
  - marketing.skill.progress
approval_policy: report_only
timeout: 4m
---
Crawl a site, score the highest-priority SEO issues, and explain the findings in plain language that a marketer can act on without opening raw logs.

## Inputs
- url (string, required): Canonical site or landing page URL to audit.
- sitemap_url (string, optional): Sitemap location if known.
- focus_keywords (array, optional): Keywords to use during the audit.
- locale (string, optional): Locale used for the audit.
- device (string, optional): Device context such as mobile or desktop.
- crawl_limit (number, required): Maximum pages to crawl.
- check_competitors (array, optional): Competitor URLs for comparative checks.

## Outputs
- canonical_url (string, optional): Normalized URL used for the crawl.
- score (number, required): Overall SEO health score.
- indexed_pages (number, required): Pages crawled and included in the audit.
- findings (array, required): Findings with severity, issue, and recommendation.
- recommendations (array, optional): Prioritized next steps for the operator.
- sources (array, optional): Evidence sources and crawl artifacts.

## RPC Notifications
- marketing.skill.progress
