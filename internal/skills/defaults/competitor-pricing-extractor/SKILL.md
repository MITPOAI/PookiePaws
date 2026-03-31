---
name: competitor-pricing-extractor
description: Extract public competitor pricing and summarize pressure across offers and regions.
category: pricing
version: 1.0.0
tags: [pricing, competitor, ecommerce]
tools:
  - crawl_pages
  - normalize_currency
events:
  - workflow.submitted
transport: jsonrpc
rpc_method: marketing.pricing.extract
rpc_notifications:
  - marketing.skill.progress
approval_policy: report_only
timeout: 2m
---
Collect live competitor pricing evidence, normalize offer data, and return a short narrative marketers can use in retention or campaign planning.

## Inputs
- competitor (string, required): Competitor brand or account name.
- domains (array, required): Public domains to crawl for pricing evidence.
- products (array, optional): Product or plan names to match.
- regions (array, optional): Market regions to compare.
- currency (string, optional): Currency to normalize toward.
- max_pages (number, required): Maximum pages to inspect.
- capture_promotions (boolean, optional): Include public promotions and bundle language.

## Outputs
- competitor (string, required): Competitor the extraction belongs to.
- observations (array, required): Normalized pricing observations with source links.
- summary (string, required): Pricing-pressure summary for the operator.
- sources (array, optional): Evidence sources supporting the pricing summary.

## RPC Notifications
- marketing.skill.progress
