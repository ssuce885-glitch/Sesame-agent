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
- Treat watcher or manual input as `TriggerEvent -> Incident`; do not skip incident ingest and do not launch child agents directly.
- Ask only for fields that matter to the spec and its operating boundary.
- Prefer stable watcher or external-script contracts over ad hoc shell loops.
- Place detector logic in `scripts/detect.sh`, not inline shell prose inside the prompt.
- Place child-agent strategy assets in `child_agents/<phase>/<agent_id>/strategy.json`, `prompt.md`, and `skills.json`.
- If the user asks for email, Feishu, or similar notification, select that as an external skill and record it in `skills.json.required`; do not describe it as a built-in runtime notification channel.
- Assume `automation_apply` will auto-install and start the watcher when the spec state is `active`.
- If the user wants the automation to start now, the draft must already contain at least one runnable watcher signal, not only abstract intent text.
- Hand off to `automation-standard-behavior` for the final confirmation summary before apply.

## Never Do

- Do not propose `while true`, infinite polling shells, or background shell hacks as the automation implementation.
- Do not redirect the user toward `schedule_report`, `task_create`, or direct child-agent launch for automation setup.
- Do not treat an automation request as a normal one-shot repair task.
- Do not suggest any direct-run backdoor for manual testing; synthetic triggers are the only supported test path.
