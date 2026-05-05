---
id: workflow-template-curator
name: workflow-template-curator
version: 0.1.0
description: Curate workflow template drafts and keep template metadata consistent with runtime constraints.
scope: workspace
requires_tools:
  - file_read
  - file_write
  - glob
risk_level: medium
approval_required: false
prompt_file: prompt.md
examples:
  - examples/review-library.md
tests:
  - tests/review-library.md
permissions:
  write_workspace: true
  external_send: false
when-to-use:
  - Review workflow template JSON files and library metadata together.
---
Keep workflow template edits scoped to `examples/workflows/` and adjacent documentation.

Do not introduce automatic loading behavior. Update indexes and descriptions so human operators can distribute templates intentionally.
