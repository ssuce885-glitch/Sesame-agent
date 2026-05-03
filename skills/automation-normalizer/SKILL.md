---
name: automation-normalizer
description: Normalize simple automation definitions, watcher contracts, routing defaults, and rejection rules
triggers:
  - "normalize automation"
  - "watcher contract"
  - "automation defaults"
allowed_tools:
  - file_read
  - file_write
policy:
  allow_implicit_activation: true
  capability_tags:
    - automation
    - validation
    - watcher
---

# Automation Normalizer

Use this skill when preparing or reviewing a simple automation definition.

## Required Fields

- `title`: short human-readable automation name.
- `goal`: what the owner role should do when the watcher matches.
- `watcher_path`: workspace-relative watcher script path.
- `watcher_cron`: cron schedule. Use an explicit value; default to `*/5 * * * *` only when the user has not specified one.

## Watcher Contract

Watcher scripts should emit a compact JSON signal. A `needs_agent` signal must include enough summary and dedupe information for the runtime to decide whether to dispatch a role task.

Expected status values:

- `idle`: no task needed.
- `needs_agent`: dispatch the owning role task.
- `error`: watcher failed before a decision.

## Routing Defaults

- Automation owner must be `role:<role_id>`.
- Automation task reports go to the main agent report stream.
- Do not route reports back to the owner role.

## Rejection Rules

- Reject `owner: main_agent` for simple automations.
- Reject role report targets for simple automations.
- Reject watcher definitions that do not produce a stable dedupe key.
- Reject automation definitions that combine watcher creation with immediate business execution.
