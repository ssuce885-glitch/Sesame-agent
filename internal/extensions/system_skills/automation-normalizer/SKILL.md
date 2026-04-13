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
- `signals[].selector = "<shell command to inspect state>"`
- `signals[].payload.interval_seconds`
- `signals[].payload.trigger_on`
  Supported values: `nonzero_exit`, `stdout_contains`, `stderr_contains`, `output_contains`
- `signals[].payload.signal_kind`
- `signals[].payload.timeout_seconds` when the check should not run forever
- Optional: `signals[].payload.summary`, `working_dir`, `cooldown_seconds`, `match`, `env`

If the task should be saved without starting, use a paused automation instead of inventing a fake watcher.

## Hard Rules

- Missing certainty becomes an explicit item in `assumptions`.
- Main-session delivery stays out-of-band through notice-first runtime reporting; never write the final result directly into the active main chat turn.
- Long-running behavior belongs only to runtime-managed watchers or external scripts.
- Child agents are one-shot execution units, not long-lived workers.
- Never place skill names, prompts, or session ids into trigger payload assumptions.
- Do not call `automation_apply` with an active automation that has no runnable watcher signal, because runtime install will fail.
- Do not call `automation_apply` before the user has explicitly confirmed the final summary.

## Rejection Rules

- Reject specs that still depend on fake shell loops or unmanaged background processes.
- Reject specs that skip `response_plan`, `delivery_policy`, or `runtime_policy`.
