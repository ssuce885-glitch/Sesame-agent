# Feature: Claude-style tool runtime alignment

## Goal
Continue aligning Sesame-agent's tool runtime contract with the Claude Code-style model of typed input, structured internal output, and mapped provider-facing output.

## Current State
- Core runtime already supports `ExecuteRich`, structured data, model result mapping, and tool-run persistence.
- Remaining built-ins were migrated to typed input/output and output schemas.
- Focus now shifts from contract shape to rollout quality: provider propagation, resource-aware concurrency, and cleanup of compatibility paths.

## Scope
- Tighten internal runtime expectations for tool execution and result mapping
- Improve provider-facing structured result propagation where low risk
- Add resource-aware concurrency rules for mutating tools touching the same target
- Keep wire compatibility while reducing ad-hoc result handling

## Acceptance Criteria
- Tool runtime behavior is documented in code and exercised by focused tests
- Current structured tool outputs continue to pass focused runtime/engine/API tests
- A concrete next-step breakdown exists for provider propagation and concurrency tightening
- Trellis context for this repository points at the real Sesame-agent module layout

## Out of Scope
- Large UI redesign
- Full removal of legacy compatibility fields in one step
- Broad repository refactors unrelated to runtime/tool execution
