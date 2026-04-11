# Skill Normalizer

Use this skill when a third-party skill source needs to be adapted into Sesame's installable format.

Sesame skill sources must be directories that contain both:

- `SKILL.json` for machine-readable metadata
- `SKILL.md` for the human-readable skill body

Normalization rules:

1. Preserve the skill's behavior and instructions.
2. Move metadata such as name, description, and enablement into `SKILL.json`.
3. Keep instruction content in `SKILL.md` without YAML frontmatter.
4. Do not install or mirror the skill into `.claude`, `.codex`, `.cursor`, or other non-Sesame platform directories.
5. Prefer Sesame-native source paths such as `.sesame/skills/...`, `.agents/skills/...`, `skills/...`, or `marketplace/skills/...`.

When converting a source:

1. Inspect the existing directory for metadata and markdown content.
2. Create or update `SKILL.json` with the normalized metadata.
3. Keep `SKILL.md` focused on the instructions the model should read at runtime.
4. Validate that both files exist before handing the source to `sesame skill install`.
