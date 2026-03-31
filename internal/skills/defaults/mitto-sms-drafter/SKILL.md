---
name: mitto-sms-drafter
description: Draft SMS campaigns for operator review before handoff to Mitto.
tools:
  - draft_sms
  - validate_recipients
events:
  - workflow.submitted
---
Draft a message, report delivery issues early, and emit an approval-gated send intent for the Mitto adapter.
