---
name: automation-intake
description: Use when the runtime has already classified the turn as automation creation or automation management instead of direct execution.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
---

# Automation Intake

Legacy wrapper for `automation-standard-behavior`.

Use this when the runtime is already in the automation compile path and you specifically need the intake phase.

## Rules

- Produce or refine an `AutomationSpec` draft before any install or apply step.
- Keep the task in the automation lifecycle: draft, normalize, dispatch-plan, confirm, apply, manage.
- Ask only for fields that matter to the spec and its operating boundary.
- Prefer stable watcher or external-script contracts over ad hoc shell loops.
- Assume `automation_apply` will auto-install and start the watcher when the spec state is `active`.
- If the user wants the automation to start now, the draft must already contain at least one runnable watcher signal, not only abstract intent text.
- Hand off to `automation-standard-behavior` for the final confirmation summary before apply.

## Never Do

- Do not propose `while true`, infinite polling shells, or background shell hacks as the automation implementation.
- Do not redirect the user toward `schedule_report`, `task_create`, or direct child-agent launch for automation setup.
- Do not treat an automation request as a normal one-shot repair task.
