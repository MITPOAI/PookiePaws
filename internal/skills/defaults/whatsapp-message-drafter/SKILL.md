---
name: whatsapp-message-drafter
description: Draft approval-gated outbound WhatsApp sends for a single recipient.
tools:
  - draft_whatsapp
  - validate_whatsapp_recipient
events:
  - workflow.submitted
---
Prepare a WhatsApp text or template send, keep it visible for operator approval, and hand the outbound action to the configured channel adapter.
