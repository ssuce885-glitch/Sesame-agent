# Automation Skill Baseline

This document defines the baseline behavior expected from the project's automation system skills.

The runtime should rely on two automation skills only:

- `automation-standard-behavior`
- `automation-normalizer`

## Intent

The goal is to keep simple-chain automation behavior stable, auditable, and mode-safe.

The simple chain is:

`watcher signal -> owner task -> report`

## Mode Boundaries

### Create Automation

This mode is for defining, updating, or replacing an automation.

- Gather only automation-definition inputs.
- Use automation tools for create/update/query work.
- Do not execute the business action early.
- If a role is intended to own the automation, that role should create it unless the user explicitly asks for a different flow.

### Owner Task

This mode is for runtime execution after a watcher match.

- Execute the business action defined by `automation_goal`.
- Report the result in the requested format.
- Do not create or modify automations here.
- Do not repair watcher scripts or role configuration here.

### Status/Report

This mode is for inspection, explanation, and progress reporting.

- Read current state.
- Report the current state.
- Identify mismatches clearly.
- Do not mutate automation definitions or watcher scripts unless the user explicitly asked for a repair.

## Hard Constraints

- `main_agent` must not silently create an automation that should be created by the owning role.
- Owner-task execution must not drift into configuration work.
- Status/report turns must not drift into repair work.
- Watcher definitions must match the runtime's actual signal contract.
- Routing and policy defaults must be explicit and stable.

## Two-Skill Model

### `automation-standard-behavior`

This skill owns:

- workflow framing
- mode identification
- cross-mode prohibitions
- creation vs execution vs report boundaries

### `automation-normalizer`

This skill owns:

- required fields
- watcher contract
- routing defaults
- policy defaults
- assumptions
- rejection rules

## Why This Exists

The previous four-skill split created too much overlap:

- `automation-standard-behavior`
- `automation-intake`
- `automation-normalizer`
- `automation-dispatch-planner`

That overlap made it easier for the model to mix modes, mis-route supervision, and accept watcher payloads that did not match the runtime contract.

The new two-skill baseline reduces ambiguity:

- one skill for behavior and boundaries
- one skill for normalization and hard rules
