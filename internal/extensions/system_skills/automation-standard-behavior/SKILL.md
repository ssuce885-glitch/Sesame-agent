---
name: automation-standard-behavior
description: Use when a user is defining or managing a long-running simple-chain automation.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
  capability_tags:
    - automation_standard_behavior
---

# Automation Standard Behavior

Treat this turn as simple-chain automation creation or management, not as a one-shot execution turn.

Keep the simple chain explicit:

`watcher signal -> pause watcher -> owner task -> report -> policy-driven watcher resume`

## Workflow

1. Identify the current mode before taking action.
2. Stay inside that mode's boundary.
3. Gather only the inputs needed for the simple chain.
4. Create or update using `automation_create_simple` when the turn is actually an automation-definition turn.
5. Summarize trigger signal, owner routing, report target, escalation target, and simple policy in user language.
6. Verify runtime-visible state using `automation_query` when stored spec, watcher state, or recent heartbeats matter.

## Modes

### 1. Create Automation Mode

Use this mode when the user is defining, updating, replacing, or explicitly asking for automation creation.

- Gather only fields that affect simple-chain behavior.
- Prefer `automation_create_simple` for user-facing simple automations.
- Keep detector logic in the watcher script or watcher command contract, not in improvised shell prose.
- If the user wants the automation to start immediately, require a runnable watcher command and interval now.
- If a role is meant to own the automation, delegate to that owning role and have that role create it. Do not create role-owned automations from `main_agent`.

### 2. Owner Task Mode

Use this mode when the role or owner is executing after a watcher match.

- Execute the business action defined by `automation_goal`.
- Report the result in the requested format.
- Stay on the execution path; do not turn owner-task execution into automation-definition work.
- Do not call `automation_create_simple`, edit watcher scripts, or update role configuration from Owner Task Mode.

### 3. Status/Report Mode

Use this mode when the user asks for current status, progress, explanation, or diagnosis.

- Read current state.
- Summarize what exists now.
- Identify blockers or mismatches clearly.
- Do not mutate automation state just because a problem was discovered.

## Hard Rules

- Treat automation work as automation work, not as a normal one-shot repair turn.
- Prefer script-backed watcher signals over ad hoc shell loops or unmanaged background processes.
- Use automation tools for simple create, lookup, control, and read-only query.
- Do not substitute `schedule_report`, `task_create`, or direct background shells for automation creation.
- Keep owner-task flow explicit; do not route through managed incident/dispatch planning.
- Do not recommend `automation_apply` or managed detector/incident/dispatch builder tools.
- Do not propose `while true`, infinite polling shells, or background shell hacks as the automation implementation.
- Simple watchers pause after a dispatch; owner task completion resumes the watcher only when `simple_policy` says to continue.

## Cross-Mode Prohibitions

- Do not let `main_agent` create an automation that should be created by the owning role.
- Do not let owner-task execution drift into editing automation definitions, watcher scripts, or role configuration.
- Do not let a status/report turn drift into "fixing" the automation unless the user explicitly asked for a repair.
- Do not conflate "report the current situation" with "repair the current situation."
- Do not delegate away from owner-task execution unless the `automation_goal` explicitly requires delegation.

## Output Expectations

- Be explicit about the current mode.
- Keep owner-task flow and reporting flow easy to audit.
- When something is wrong, state the mismatch first, then state the next safe action.
