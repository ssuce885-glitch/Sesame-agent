---
name: skill-normalizer
description: Normalize downloaded or third-party skills into Sesame's canonical SKILL.md format and non-ambient runtime policy.
policy:
  allow_implicit_activation: false
  allow_full_injection: true
  preferred_tools:
    - list_dir
    - file_read
    - grep
    - apply_patch
---

# Skill Normalizer

Use this skill only when the user explicitly asks to import, normalize, adapt, or rewrite a skill for Sesame.

Goal: produce a Sesame-native skill package that is safe to install without turning the imported skill into ambient prompt pollution.

## Workflow

1. Inspect the source skill directory.
   - Confirm whether `SKILL.md` already exists.
   - Inventory `scripts/`, `references/`, and `assets/` before rewriting.
2. Rewrite the package into Sesame canonical layout:
   - root `SKILL.md`
   - optional `references/`
   - optional `scripts/`
   - optional `assets/`
3. Rewrite or synthesize frontmatter with conservative defaults:

```yaml
name: <skill-name>
description: <what the skill does>
allowed-tools: []
policy:
  allow_implicit_activation: false
  allow_full_injection: true
  capability_tags: []
  preferred_tools: []
```

4. Deplatform the instructions.
   - Remove or rewrite `.claude`, `.codex`, `.cursor`, `.windsurf`, `.opencode`, `.gemini`, and similar runtime-specific paths.
   - Remove slash-command syntax or permission language that only makes sense in another agent runtime.
5. Translate tool guidance into Sesame semantics.
   - Prefer Sesame tool names only when the mapping is clear.
   - If the skill asks the model to execute commands, scripts, patches, browser actions, or task orchestration, add the matching structured tool declarations instead of leaving that requirement only in prose.
   - Use `allowed-tools` for required execution tools.
   - Use `policy.preferred_tools` when one tool is the clear primary path.
   - Keep purely cognitive or policy skills tool-free unless they truly need execution access.
   - Do not invent capability tags, allowed tools, or preferred tools without evidence from the source skill body, bundled scripts, or explicit examples.
6. Keep `SKILL.md` concise.
   - Move bulky examples, schemas, or vendor docs into `references/`.
   - Keep operational scripts in `scripts/`.
7. Present the normalized result or diff before installing when the source is ambiguous or risky.

## Hard Rules

- Do not enable `allow_implicit_activation` by default for imported skills.
- Do not preserve instructions that tell the model to probe unrelated local environments.
- Do not assume browser automation is ambiently available.
- Do not silently rewrite unrelated installed skills during other tasks.
- Do not leave execution instructions only in prose. If the normalized skill tells the model to run a Sesame tool, command, script, patch, browser action, or task workflow, declare the needed tools structurally.
- Do not add `capability_tags` unless the runtime meaning is explicit and high-confidence.

## Normalization Heuristics

- Keep the original skill intent while stripping platform glue and repeated boilerplate.
- If a foreign skill mixes multiple runtimes or modes, rewrite it into concise Sesame instructions plus references instead of copying the full body verbatim.
- If a tool mapping is unclear, keep the instruction generic rather than naming the wrong Sesame tool.
- If the normalized body says "run this command" or "use this tool", the frontmatter should usually reflect that with `allowed-tools` and, when appropriate, `policy.preferred_tools`.

## Expected Output

Return:

- what was kept
- what was rewritten
- any assumptions or missing pieces before install
- if the normalized package is written into a Sesame install root, note that it should become visible on the next turn without requiring a restart
