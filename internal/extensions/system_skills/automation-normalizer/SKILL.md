---
name: automation-normalizer
description: Use when a simple-chain automation draft needs defaults, assumptions, and hard-rule normalization before create/update.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
---

# Automation Normalizer

Normalize simple-chain automation inputs before persisting with `automation_create_simple`.

## Required Fields

- `automation_id`
- `owner`
- `watch_script`
- `interval_seconds`

## What To Produce

- normalized `owner`
- normalized `report_target`
- normalized `escalation_target`
- normalized `simple_policy`
- `watcher_lifecycle` with `mode=continuous` and `after_dispatch=pause`
- explicit `assumptions`

## Runnable Watcher Contract

For an active simple automation, the watcher script input must be runnable and bounded:

- `watch_script` is a concrete command or script invocation
- `interval_seconds` is a positive poll interval
- Optional `timeout_seconds` is set when the check should not run indefinitely

For script-backed simple watcher signals, the watcher output must match the runtime's expected signal contract.

- The watcher stdout must be exactly one JSON object.
- Required fields:
  - `status`: one of `healthy`, `recovered`, `needs_agent`, `needs_human`
  - `summary`: a non-empty human-readable string
- Optional fields:
  - `facts`: JSON object with detector facts
  - `actions_taken`: array of strings
  - `hints`: array of strings
  - `dedupe_key`: string
- `needs_agent` and `needs_human` outputs must include a non-empty stable `dedupe_key`.
- The `dedupe_key` identifies one real-world signal that should dispatch at most one owner task. Re-running the watcher for the same incident, file version, feed item, or scheduled slot must produce the same key.
- Good `dedupe_key` sources include a schedule bucket (`daily-report:2026-04-27:04Z`), source item id or URL, file path plus mtime/content hash, error signature plus time bucket, or status-page incident id.
- Do not use random ids, process ids, current seconds, full timestamps, attempt counters, or changing summaries as `dedupe_key`.
- `healthy` outputs may omit `dedupe_key` or leave it empty.
- Use `healthy` when no owner task should run.
- Use `needs_agent` when the owner role should execute the automation task.
- Example no-match output:
  `{"status":"healthy","summary":"no .txt files found","facts":{"count":0}}`
- Example match output:
  `{"status":"needs_agent","summary":"found .txt files to clean","dedupe_key":"box-cleaner:txt-files:2026-04-27T04","facts":{"count":2}}`
- Do not accept ad hoc detector payloads that use a made-up schema.
- Do not wrap the payload inside `script_status`.
- Do not use `triggered`, `found`, `match`, `no_match`, `TRIGGER`, or `NO_MATCH` as the top-level protocol.
- Do not accept drafts that rely on a legacy `{"trigger": ...}` style payload when the runtime expects a structured `script_status` result.

## Hard Rules

- Missing certainty becomes an explicit item in `assumptions`.
- Long-running behavior belongs only to runtime-managed watchers or external scripts.
- Owner routing must stay in simple form: `main_agent` or `role:<role_id>`.
- Keep routing explicit and simple: owner executes, report target is explicit, escalation target is explicit when supervision should not rely on defaults.
- Normalize policy choices to the simple policy envelope (`on_success`, `on_failure`, `on_blocked`) using only `continue`, `pause`, or `escalate`.
- Default simple policy to: success=`continue`, failure=`pause`, blocked=`escalate`.
- Default watcher lifecycle to dispatch-once-per-cycle: `mode=continuous`, `after_dispatch=pause`. The runtime resumes the watcher after the owner task only when policy allows it.
- `on_success=continue` means resume the watcher for future signals; it must not rely on changing or missing `dedupe_key` to re-dispatch the same signal.
- Use explicit report and escalation targets when the user asks for non-default behavior.
- Prefer explicit assumptions over pretending domain certainty that does not exist.
- If the parent or main agent is expected to supervise the flow, do not trap reporting inside the same specialist role by accident.

## Rejection Rules

- Reject specs that still depend on fake shell loops or unmanaged background processes.
- Reject drafts with an invalid owner target or missing watcher command.
- Reject watcher definitions that are not runnable now when the automation is expected to start immediately.
- Reject watcher outputs that do not match the runtime-visible signal contract.
- Reject ambiguous drafts that mix creation intent, owner-task execution intent, and status/report intent in one step without an explicit transition.
