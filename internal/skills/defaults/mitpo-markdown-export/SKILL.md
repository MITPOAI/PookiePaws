---
name: mitpo-markdown-export
description: Save text content to the workspace as a timestamped Markdown file.
category: export
version: 1.0.0
tags: [export, markdown, file]
tools:
  - write_file
events:
  - workflow.submitted
approval_policy: report_only
timeout: 30s
---
Write provided text content to a timestamped Markdown file inside workspace/exports/.

## Inputs
- content (string, required): Text content to export as Markdown.
- title (string, optional): Title for the Markdown document header.
- filename (string, optional): Custom filename prefix (defaults to "export").

## Outputs
- path (string, required): Absolute file path of the exported Markdown document.
- size (number, required): Size of the written file in bytes.
