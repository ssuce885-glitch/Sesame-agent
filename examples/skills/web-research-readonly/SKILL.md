---
id: web-research-readonly
name: web-research-readonly
version: 0.1.0
description: Analyze locally captured web material and research notes without performing live external fetches.
scope: workspace
requires_tools:
  - file_read
  - grep
  - load_context
risk_level: low
approval_required: false
examples:
  - examples/research-brief.md
tests:
  - tests/research-brief.md
permissions:
  network_read: false
  external_send: false
triggers:
  - Summarize imported research artifacts that already exist in the workspace.
---
Work only from material already present in the workspace, report archive, or context store.

Do not browse, fetch, or send results to external systems. Surface gaps clearly when the local snapshot is stale or incomplete.
