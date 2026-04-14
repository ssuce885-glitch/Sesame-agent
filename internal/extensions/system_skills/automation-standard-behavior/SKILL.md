---
name: automation-standard-behavior
description: Use when a user is defining or managing a long-running automation that needs a script-backed AutomationSpec and confirmation before apply.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
  capability_tags:
    - automation_standard_behavior
---

# Automation Standard Behavior

Treat this turn as automation compilation or automation management, not as a one-shot execution turn.

## Workflow

1. Draft the script-backed automation first.
2. Keep detector logic in `scripts/detect.sh` and keep child-agent strategy assets in `child_agents/<phase>/<agent_id>/strategy.json`, `prompt.md`, and `skills.json`.
3. Normalize the `AutomationSpec` so the watcher signal, response plan, delivery policy, runtime policy, and assumptions are explicit.
4. Treat `response_plan.child_agents[].prompt_template` and `activated_skill_names` as preview/cache fields; runtime execution reads the workspace asset bundle as the canonical source.
5. Summarize trigger, interval, remediation path, escalation path, selected external skills, and assumptions in user language.
6. Wait for explicit user confirmation.
7. Call `automation_apply` only with `confirmed=true`.

## Hard Rules

- Prefer script-backed watcher signals over ad hoc shell loops or unmanaged background processes.
- Use automation tools for apply, lookup, control, and incident inspection.
- Do not substitute `schedule_report`, `task_create`, or direct background shells for automation creation.
- If the automation should start immediately, the draft must already contain a runnable watcher signal and any referenced script assets.
- Detector scripts emit facts, attempted fixed actions, and escalation status; they do not contain the full child-agent prompt.
- Persist child-agent template assets under `child_agents/<phase>/<agent_id>/strategy.json`, `prompt.md`, and `skills.json`.
- `skills.json.required` is the runtime contract for the external capabilities the child agent must receive.
- Use `signals[].payload.trigger_on = "script_status"` for script-backed detector assets.
- Child agents are one-shot responders after a trigger or incident, not long-lived workers.
- Persist `response_plan` in the versioned `schema_version = "sesame.response_plan/v2"` shape.
- Structure `assumptions` as `{field,value,reason}` objects rather than free-form strings.
- Manual testing must emit a synthetic trigger through the normal ingest path; never propose or rely on `automation run`.
- Approval-capable child-agent templates require `runtime_policy.approval_binding.workspace_binding` and `owner_key`.
- Notification is never a built-in runtime channel here; if the user asks for email or Feishu, choose the matching external skill and persist it in `skills.json`.
- Final results return through notice plus mailbox/reporting; next-turn injection is opt-in and should stay summary-only.
