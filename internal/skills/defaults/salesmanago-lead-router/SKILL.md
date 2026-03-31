---
name: salesmanago-lead-router
description: Route inbound CRM leads to the correct queue with approval-aware write intents.
tools:
  - route_lead
  - classify_segment
events:
  - workflow.submitted
---
Turn lead routing rules into a dry-run Salesmanago action that can be approved by an operator before execution.
