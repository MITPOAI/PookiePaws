---
name: cross-channel-campaign-drafter
description: Draft coordinated campaign assets across channels with an approval-first posture.
category: campaign
version: 1.0.0
tags: [campaign, copywriting, orchestration]
tools:
  - draft_copy
  - align_channels
events:
  - workflow.submitted
transport: jsonrpc
rpc_method: marketing.campaign.draft
rpc_notifications:
  - marketing.skill.progress
approval_policy: review_before_publish
timeout: 3m
---
Turn a campaign brief into coordinated assets for multiple channels, keeping the tone empathetic, the CTA consistent, and the final handoff approval-ready.

## Inputs
- brief (string, required): Campaign brief in marketer language.
- objective (string, required): Business or campaign objective.
- audience_segments (array, required): Audience segments with need states and offers.
- channels (array, required): Channels to draft for.
- brand_voice (string, optional): Brand voice guidance.
- tone (string, optional): Desired emotional tone.
- offer (string, optional): Offer or promotion to foreground.
- locales (array, optional): Locale variations to include.
- constraints (object, optional): Delivery constraints such as claim limits or CTA requirements.
- include_approval_step (boolean, optional): Whether the workflow should surface an approval checkpoint.

## Outputs
- summary (string, required): High-level campaign summary for the operator.
- assets (array, required): Drafted channel assets.
- journey (array, optional): Sequenced campaign journey steps.
- approval_checklist (array, optional): Human review checklist before launch.

## RPC Notifications
- marketing.skill.progress
