# Context Review Checklist

Use this before and after changes that affect prompts, memory, roles, tasks, workflows, automation state, or context preview.

## Before Editing

- Identify the execution scope: main, role, or task.
- Identify whether the data is authority, dashboard state, memory, report, trace, or raw conversation.
- Read `../backend/context-system.md` for visibility and injection contracts.
- Read `../backend/storage-and-migrations.md` for schema/repository changes.

## During Review

- Does main see only the supervision view of role work, not role internals?
- Does role see its own workbench and relevant workspace context, not another role's private work?
- Does task-only data stay limited to the current task lineage?
- Does `load_context` check visibility even when the caller knows the ID?
- Is limit applied after visibility filtering?
- Does prompt text say Runtime State is a dashboard, not a rule source?
- Are `AGENTS.md` conflicts treated as current-turn overrides plus a user-facing update question, not silent durable rule changes?

## Required Test Thinking

- Add one positive test for the expected visible path.
- Add one negative test for the hidden or conflicting path.
- Prefer package-level tests close to the boundary being changed: `agent`, `contextsvc`, `tools`, `store`, or `contextasm`.

## Stop Conditions

Pause implementation and clarify design if a change would:

- Make memory globally visible by default from role/task execution.
- Promote role-only/task-only data to workspace without main/user action.
- Treat Runtime State, Memory, Report, or ContextBlock as higher authority than `AGENTS.md`.
- Add broad fallback behavior that hides a failed visibility or migration path.
