---
id: automation-normalizer
name: automation-normalizer
version: 0.1.0
description: Normalize watcher or automation signals into concise operator-ready summaries.
scope: system
requires_tools:
  - automation_query
  - file_read
  - file_edit
risk_level: medium
approval_required: false
examples:
  - examples/normalize-signal.md
tests:
  - tests/normalize-signal.md
permissions:
  write_workspace: true
  external_send: false
triggers:
  - Normalize noisy automation payloads before handoff.
policy:
  allow_implicit_activation: true
---
Turn watcher payloads, automation records, and captured notes into a normalized summary with stable sections for signal, evidence, impact, and next action.

Do not trigger external delivery. Limit edits to workspace draft artifacts or structured notes.
