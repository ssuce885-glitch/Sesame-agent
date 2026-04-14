---
name: automation-dispatch-planner
description: Use when a normalized AutomationSpec still needs standard response_plan and delivery_policy values without domain-specific remediation logic.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
---

# Automation Dispatch Planner

Legacy wrapper for `automation-standard-behavior`.

Fill the standard dispatch shape for automation without introducing domain-specific repair logic.

## What To Produce

- `response_plan`
- `delivery_policy`

## Planning Rules

- `response_plan` must use the versioned multi-phase runtime shape and describe when a one-shot child agent is allowed after a trigger or incident.
- `delivery_policy` must describe how results return through notice-first runtime reporting, and when escalation or human review is required.
- Prefer explicit assumptions over pretending domain certainty that does not exist.
- Keep the plan generic enough that later domain skills can attach container, database, batch-job, or project-specific behavior on top.
- When the automation should start immediately, make sure the normalized spec already has a runnable watcher signal because `automation_apply` will install it right away.
- The final plan still requires an explicit user confirmation summary before `automation_apply`.
- Delivery defaults are mailbox-first and notice-enabled; next-turn injection stays disabled unless the user explicitly opts in.
- Templates that may need elevated permissions must say so up front so the runtime can route approval through the workspace-level pending queue.

## Never Do

- Do not hardcode container, database, or application-specific remediation steps here.
- Do not allow trigger payloads to choose skills, prompts, or sessions.
- Do not bypass `automation_apply` after the dispatch plan is produced.
- Do not add a second automation-only result-reading protocol; mailbox/reporting stays canonical.
