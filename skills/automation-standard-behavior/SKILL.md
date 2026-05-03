---
name: automation-standard-behavior
description: Stable behavior boundaries for simple watcher-based automations
triggers:
  - "create automation"
  - "automation status"
  - "watcher signal"
  - "owner task"
allowed_tools:
  - automation_create_simple
  - automation_query
  - automation_control
  - file_read
policy:
  allow_implicit_activation: true
  capability_tags:
    - automation
    - scheduler
    - role-owned
---

# Automation Standard Behavior

Use this skill when creating, running, or reporting on simple automations.

Simple automation flow:

```text
watcher signal -> owner role task -> main agent report
```

## Create Automation

- Collect only the fields needed to define the automation.
- Use automation tools to create, update, and query automation state.
- Do not run the business task while defining the automation.
- If a role should own the automation, that role should create it unless the user explicitly asks for another flow.

## Owner Task

- Execute the `automation_goal` business action.
- Return the result as the owner role's final answer; runtime reports it to the main agent.
- Do not call `delegate_to_role` to report back to the main agent.
- Do not create or edit automation definitions from an owner task.
- Do not edit watcher scripts or role configuration from an owner task.

## Status And Report

- Read and report current state.
- Point out mismatches, blockers, and recent failures clearly.
- Do not modify automation definitions or watcher scripts unless the user explicitly asks.

## Hard Rules

- `main_agent` must not silently create automation assets that should be owned by a specialist role.
- Owner-task execution must not drift into configuration work.
- Status/report turns must not drift into repair work.
- Watcher definitions must match the runtime signal contract.
- Routing and policy defaults must be explicit and stable.
