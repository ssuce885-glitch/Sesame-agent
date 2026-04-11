# Sesame Skill/Provider Runtime Hard Reset Design

## Goal

Replace Sesame's current mixed provider inference, implicit skill activation, and soft tool-routing behavior with a hard-reset architecture:

- model/provider selection is explicit
- skills are metadata-first and explicitly invoked
- installed skills are trusted and may extend tools for the current turn
- no legacy config or legacy skill compatibility paths remain

This design intentionally breaks historical behavior to remove technical debt instead of preserving it.

## Non-Goals

- Backward compatibility for old config layouts
- Backward compatibility for single-file or frontmatter-only skill formats
- Automatic trigger-based skill activation
- Treating capability profiles as security boundaries

## Chosen Direction

This design adopts a trusted-install overlay model:

- the model sees skill metadata and can proactively call `skill_use`
- `skill_use` injects the skill body and extends the turn's visible tools
- install is the only trust boundary for local skills

This keeps the interaction model close to Claude Code while allowing higher automation than Claude Code's stricter tool boundary model.

## Current Problems

### Provider/runtime problems

- provider selection currently depends on `compat_mode`, base URL heuristics, and implicit fallbacks
- provider profile selection is inferred from URL shape instead of explicit configuration
- config validity is only partially checked before runtime
- model availability errors are surfaced too late

### Skill/runtime problems

- skill discovery, activation, prompt injection, and tool expansion are coupled
- skill activation can happen implicitly from string matching
- skill bodies and runtime metadata are mixed in the same parsing pipeline
- tool grants are derived from multiple overlapping fields
- `enabled` only partially affects runtime behavior

## Architecture Overview

The new runtime has four explicit layers:

1. Provider registry
2. Profile selection
3. Skill catalog metadata
4. Turn-local skill overlays

At turn start, the runtime resolves a profile, computes the base tool set, loads only skill metadata, and injects a catalog section into the prompt. No skill is active yet.

If the model calls `skill_use`, the runtime validates the skill, loads `SKILL.json` and `SKILL.md`, injects the skill instructions, computes a tool overlay from `tool_dependencies`, and recompiles the visible tool set for the rest of the turn.

## Provider Configuration Model

### Structure

Runtime config is split into:

- `model_providers`
- `profiles`
- `active_profile`

Each provider entry defines transport and authentication facts. Each profile defines a runnable model selection against one provider entry.

### Provider entry

Each provider definition must include:

- `id`
- `api_family`
- `base_url`
- `api_key_env` or equivalent auth source
- optional provider-specific runtime flags
- optional explicit provider profile identifier

### Profile entry

Each profile must include:

- `model`
- `model_provider`
- optional reasoning settings
- optional verbosity settings
- optional cache profile
- optional runtime defaults that belong to model execution

### Runtime resolution

Runtime startup must:

1. resolve the active profile
2. load the referenced provider definition
3. validate the combined configuration
4. build a `ResolvedProviderConfig`
5. construct the transport

There is no provider inference on the main path. Base URL inference, compat mode mapping, and implicit provider fallback are deleted.

### Failure model

If the active profile is missing, the provider is missing, auth fields are missing, or the provider entry is invalid, startup fails with a direct error. The runtime does not silently downgrade.

## Skill Format

### Directory contract

Every valid skill directory must contain:

- `SKILL.json`
- `SKILL.md`

Optional directories such as `scripts/`, `assets/`, and `references/` are allowed but not required.

### `SKILL.md`

`SKILL.md` contains only the model-facing instructions for the skill. It is not parsed for routing, tool grants, or activation hints.

### `SKILL.json`

`SKILL.json` is the single source of truth for runtime metadata.

Required fields:

- `name`
- `description`

Optional fields:

- `when_to_use`
- `tool_dependencies` (default `[]`)
- `preferred_tools` (default `[]`)
- `execution_mode`
- `agent`
- `env_dependencies` (default `[]`)
- `enabled` (default `true`)

Install scope is runtime-derived from the skill root where the skill was discovered. It is not declared by the skill author inside `SKILL.json`.

### Metadata semantics

- `tool_dependencies` is an exact list of canonical tool identifiers known to the runtime tool registry
- `preferred_tools` is advisory prompt metadata only and never grants access by itself
- `env_dependencies` describes activation-time prerequisites only; Sesame may validate and report them, but does not auto-install or auto-enable anything from them

### Removed concepts

These concepts are deleted:

- markdown frontmatter skill metadata
- structured markdown parsing as skill metadata
- `allowed-tools`
- `policy.allow_implicit_activation`
- trigger phrase matching
- body-driven activation semantics
- grant aggregation from mixed fields such as `Agent.Tools` and `AllowedTools`

## Skill Trust Model

Installation is the only trust boundary.

- if a skill is installed in Sesame-managed skill roots, it is trusted
- if it is trusted and enabled, `skill_use` may extend the turn tool set from `tool_dependencies`
- there is no second approval state
- there is no "installed but untrusted" state

This is intentionally high-automation and assumes local skill installation is an operator action with full trust implications.

## Turn Runtime Model

### Turn start

At the start of each turn the runtime must:

1. resolve the active profile
2. compute base visible tools from the profile
3. load the skill catalog metadata
4. inject a catalog section describing available skills
5. start with an empty `active_skills` set

No skill is active by default. No skill body is injected by default.

### Skill activation

When the model calls `skill_use(name)`:

1. resolve the named skill from the installed catalog
2. verify the skill is enabled
3. load and validate `SKILL.json`
4. verify `tool_dependencies` only references known tools
5. verify declared `env_dependencies`
6. load `SKILL.md`
7. add the skill to `active_skills`
8. merge `tool_dependencies` into the current turn overlay
9. rebuild visible tools for the rest of the turn
10. rebuild prompt injections for the rest of the turn

### Tool visibility

Final turn-visible tools are:

`base profile visible tools + union(active skill tool_dependencies)`

Because install is the trust boundary, skill overlays may override profile-hidden tools. Capability profiles remain useful for default routing and default tool exposure, but they are no longer the final authority once a trusted skill is activated.

### Multiple skills

Multiple `skill_use` calls in a turn are allowed. Their overlays are unioned. Duplicate activations are ignored idempotently.

### End of turn

Active skills and tool overlays expire at turn end. They do not implicitly carry across turns.

If the next turn needs a skill again, the model calls `skill_use` again.

### Child tasks and child agents

Subtasks inherit only the explicit active skill set of the parent turn, never inferred names or suggestions. Each child execution rebuilds its own overlay from that explicit set.

## Prompt/Instruction Model

The instruction compiler handles only two skill-related prompt sections:

- skill catalog metadata
- active skill bodies

There is no implicit hint section, no trigger section, and no retrieval-driven activation section.

The catalog section should be concise and metadata-only. The active skill section should contain the loaded `SKILL.md` body of each activated skill plus a concise summary of newly enabled tools.

## Skill Discovery and Catalog Behavior

Catalog discovery only reads `SKILL.json` during normal listing and prompt preparation. `SKILL.md` is read only when the skill is actually activated.

Disabled skills are fully excluded from:

- prompt-visible catalog listings
- `skill_use`
- tool overlays
- env dependency application

This makes `enabled` a true runtime switch instead of a partial env-only toggle.

## Error Handling

### Invalid provider config

Startup error. No fallback.

### Invalid active profile

Startup error. No fallback.

### Invalid skill directory

Catalog error entry plus operator-visible warning. The skill is not loaded.

### Missing `SKILL.json` or `SKILL.md`

Invalid skill. No compatibility downgrade.

### `skill_use` on unknown or disabled skill

Structured tool error returned to the model and UI.

### Invalid `tool_dependencies`

Skill load failure. The skill is excluded from the catalog.

### Unmet `env_dependencies`

Structured tool error from `skill_use`. The skill is not activated and no overlay is applied.

## Migration Policy

This redesign is a hard cut, not a compatibility bridge.

### Deleted runtime inputs

These old config patterns are removed from the supported main path:

- flat provider fields as the authoritative model runtime config
- compat-mode-driven provider selection
- base-URL-driven provider inference
- implicit provider profile inference

### Deleted skill inputs

These old skill layouts are removed:

- `SKILL.md` without `SKILL.json`
- frontmatter-only metadata
- structured markdown metadata
- trigger-based activation behavior

### Operator experience

If the runtime encounters old config or old skill layouts, it must fail loudly and specifically. It must not silently reinterpret them.

The expected migration path is explicit rewrite, not transparent compatibility.

## File/Module Impact

The redesign will require replacing or heavily restructuring at least:

- `internal/config/config.go`
- `internal/config/userconfig.go`
- `internal/config/provider_inference.go`
- `internal/model/provider_resolution.go`
- `internal/model/factory.go`
- `internal/extensions/discovery.go`
- `internal/extensions/install.go`
- `internal/skills/catalog.go`
- `internal/skills/retrieval.go`
- `internal/skills/injection.go`
- `internal/tools/builtin_skill_use.go`
- `internal/tools/skill_resolution.go`
- `internal/toolrouter/router.go`
- `internal/engine/loop.go`
- `internal/instructions/compiler.go`

Some modules should likely be deleted outright rather than preserved and adapted, especially where the old behavior is centered on inference or implicit activation.

## Testing Strategy

### Provider tests

- profile lookup succeeds for valid explicit configs
- startup fails for missing provider, missing profile, missing auth, and invalid family
- runtime never uses base URL inference on the main path

### Skill catalog tests

- directories without both `SKILL.json` and `SKILL.md` are invalid
- disabled skills are absent from the visible catalog
- metadata loading does not require reading skill bodies

### `skill_use` tests

- activating a skill injects body and overlay in the same turn
- overlay expands visible tools
- overlay can override profile-hidden tools
- duplicate activation is idempotent
- activation does not persist across turns

### Prompt tests

- turn start shows catalog metadata only
- activated turns show active skill bodies only after `skill_use`
- no implicit activation hints remain in the compiled prompt

### Migration tests

- old config shapes fail with explicit messages
- old skill layouts fail with explicit messages

## Risks

### Trust expansion

This design intentionally grants large authority to installed skills. A malicious or careless installed skill can expose powerful tools to the model. That is an accepted property of the design, not an accidental side effect.

### Operational breakage

Because compatibility is removed, existing local configs and installed skills will stop working until rewritten. This is also intentional.

### Refactor breadth

This is not a local patch. It is a runtime-level redesign touching config, discovery, prompt compilation, tool routing, and turn execution. The implementation plan must sequence deletions and replacements carefully to avoid half-migrated behavior.

## Summary

Sesame should move to:

- explicit provider registry and profiles
- dual-file skills with metadata in `SKILL.json` and instructions in `SKILL.md`
- explicit model-driven `skill_use`
- turn-local trusted skill overlays for tools
- hard failure for legacy config and legacy skill layouts

This replaces the current hybrid system with a simpler and more intentional runtime model, at the cost of breaking compatibility by design.
