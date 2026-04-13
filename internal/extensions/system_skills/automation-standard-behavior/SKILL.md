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
2. Normalize the `AutomationSpec` so the watcher signal, response plan, delivery policy, runtime policy, and assumptions are explicit.
3. Summarize trigger, interval, remediation path, escalation path, and assumptions in user language.
4. Wait for explicit user confirmation.
5. Call `automation_apply` only with `confirmed=true`.

## Hard Rules

- Prefer script-backed watcher signals over ad hoc shell loops or unmanaged background processes.
- Use automation tools for apply, lookup, control, and incident inspection.
- Do not substitute `schedule_report`, `task_create`, or direct background shells for automation creation.
- If the automation should start immediately, the draft must already contain a runnable watcher signal and any referenced script assets.
- Child agents are one-shot responders after a trigger or incident, not long-lived workers.
- Persist `response_plan` in the versioned `schema_version = "sesame.response_plan/v2"` shape.
- Structure `assumptions` as `{field,value,reason}` objects rather than free-form strings.
- Manual testing must emit a synthetic trigger through the normal ingest path; never propose or rely on `automation run`.
- Approval-capable child-agent templates require `runtime_policy.approval_binding.workspace_binding` and `owner_key`.
- Final results return through notice plus mailbox/reporting; next-turn injection is opt-in and should stay summary-only.
