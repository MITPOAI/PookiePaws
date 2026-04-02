---
name: mitpo-seo-auditor
description: Analyze a URL for keyword density, technical SEO gaps, and actionable recommendations.
category: seo
version: 1.0.0
tags: [seo, audit, search]
tools:
  - crawl_site
  - evaluate_metadata
events:
  - workflow.submitted
approval_policy: report_only
timeout: 3m
---
Audit a target URL for keyword usage, metadata completeness, and technical SEO fundamentals, then return findings in marketer-friendly language.

## Inputs
- url (string, required): Target URL to audit.
- keywords (array, optional): Focus keywords for density analysis.
- crawl_limit (number, optional): Maximum pages to inspect.
- check_mobile (boolean, optional): Include mobile-friendliness checks.

## Outputs
- url (string, required): The URL that was audited.
- score (number, required): Overall SEO health score (0-100).
- findings (array, required): Issues with severity and recommendation.
- recommendations (array, optional): Prioritized next steps.
