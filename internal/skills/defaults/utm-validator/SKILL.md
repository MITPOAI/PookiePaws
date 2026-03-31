---
name: utm-validator
description: Validate and normalize campaign UTM parameters.
tools:
  - parse_url
  - normalize_query
events:
  - workflow.submitted
---
Validate a marketing URL, normalize UTM keys to lowercase, and report missing attribution fields.
