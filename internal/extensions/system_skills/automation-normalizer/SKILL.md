---
name: automation-normalizer
description: Use when an AutomationSpec draft needs defaults, assumptions, and hard-rule normalization before automation_apply.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
---

# Automation Normalizer

Legacy wrapper for `automation-standard-behavior`.

Before `automation_apply`, make sure the spec is complete enough for the standard automation contract.

## Required Fields

- `title`
- `workspace_root`
- `goal`
- `response_plan`
- `delivery_policy`
- `runtime_policy`

## Runnable Watcher Contract

For an `active` automation, include at least one runnable `poll` signal:

- `signals[].kind = "poll"`
- `signals[].selector = "automation_script"` when the watcher is script-backed
- `signals[].payload.interval_seconds`
- `signals[].payload.trigger_on`
  Supported script-backed value: `script_status`
- `signals[].payload.signal_kind`
- `signals[].payload.timeout_seconds` when the check should not run forever
- `signals[].payload.script_path = "scripts/detect.sh"` for script-backed detectors
- Optional: `signals[].payload.summary`, `working_dir`, `cooldown_seconds`, `match`, `env`

If the task should be saved without starting, use a paused automation instead of inventing a fake watcher.

## Hard Rules

- Missing certainty becomes an explicit item in `assumptions`.
- Normalize `response_plan` to `schema_version = "sesame.response_plan/v2"` before apply.
- Normalize `assumptions` to structured `{field,value,reason}` records.
- Require each child agent to have a workspace asset bundle at `child_agents/<phase>/<agent_id>/strategy.json`, `prompt.md`, and `skills.json` before apply.
- Treat `strategy.json` and `skills.json` as the runtime source of truth; `response_plan` preview fields are only compatibility caches.
- Main-session delivery stays out-of-band through notice-first runtime reporting; never write the final result directly into the active main chat turn.
- Long-running behavior belongs only to runtime-managed watchers or external scripts.
- Child agents are one-shot execution units, not long-lived workers.
- Never place skill names, prompts, or session ids into trigger payload assumptions.
- If the user asks for notification by email, Feishu, or another channel, normalize that request into an external skill in `skills.json.required`, not into a built-in runtime delivery feature.
- If any child-agent template allows elevation, require `approval_binding.workspace_binding` and `approval_binding.owner_key`.
- Do not call `automation_apply` with an active automation that has no runnable watcher signal, because runtime install will fail.
- Do not call `automation_apply` before the user has explicitly confirmed the final summary.

## Rejection Rules

- Reject specs that still depend on fake shell loops or unmanaged background processes.
- Reject specs that skip `response_plan`, `delivery_policy`, or `runtime_policy`.
- Reject specs that reference child agents but omit the required asset bundle files.
