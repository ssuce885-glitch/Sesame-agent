---
id: notification-draft
name: notification-draft
version: 0.1.0
description: Draft outbound notifications for operator review without sending them.
scope: workspace
requires_tools:
  - file_read
  - file_write
  - task_trace
risk_level: high
approval_required: true
prompt_file: prompt.md
examples:
  - examples/draft-status-update.md
tests:
  - tests/draft-status-update.md
permissions:
  external_send: false
  delivery_mode: draft_only
when-to-use:
  - Prepare operator-reviewed messages for email, chat, or incident channels.
policy:
  allow_full_injection: false
---
Produce drafts only. Store them as workspace artifacts or report text for human review.

Never claim a draft was delivered. Never call or invent send-capable connectors.
