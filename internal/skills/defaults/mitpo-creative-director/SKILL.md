---
name: mitpo-creative-director
description: Generate copy variations from a brand voice prompt and audience brief.
category: creative
version: 1.0.0
tags: [creative, copywriting, brand]
tools:
  - generate_copy
events:
  - workflow.submitted
approval_policy: report_only
timeout: 2m
---
Take a brand voice definition, target audience, and tone direction, then produce structured copy variations a marketer can review and adapt for campaigns.

## Inputs
- brand_name (string, required): Brand or product name.
- tone (string, required): Desired tone (e.g. professional, playful, bold).
- audience (string, required): Target audience description.
- content_type (string, optional): Type of content (headline, tagline, email, social).
- guidelines (string, optional): Additional brand guidelines or constraints.

## Outputs
- brand_name (string, required): The brand the copy was generated for.
- copy_variants (array, required): Generated copy variations.
- tone_analysis (string, required): Analysis of tone alignment.
- recommendations (array, optional): Suggestions for refinement.
