---
name: mitpo-researcher
description: Fetch a public URL, strip HTML, and summarize the content for marketing intelligence.
category: research
version: 1.0.0
tags: [research, competitor, web]
tools:
  - fetch_url
  - summarize
events:
  - workflow.submitted
approval_policy: report_only
timeout: 2m
---
Fetch a public web page, strip all HTML tags, and produce a structured summary a marketer can use for competitor intelligence, positioning, or content planning.

## Inputs
- url (string, required): Public URL to fetch and summarize.
- focus (string, optional): Specific area to focus the summary on (e.g. pricing, features).

## Outputs
- url (string, required): The URL that was fetched.
- title (string, optional): Page title if available.
- summary (string, required): Structured summary of the page content.
- raw_text (string, required): Cleaned plain text from the page.
