---
name: skill-installer
description: Install, inspect, list, and remove Sesame skills with the `sesame skill` CLI.
---

# Skill Installer

Use this skill when the user asks to inspect, install, update, or remove Sesame skills.

Sesame only has one install destination model:

- global skills: `~/.sesame/skills`
- workspace skills: `<workspace>/.sesame/skills`

Repository paths are only **source candidates**. They do **not** change the Sesame install destination.

Use this workflow:

1. **Direct install**
   - Use this when the source already points to a concrete skill directory containing `SKILL.md`.
2. **Inspect first**
   - Use this when the user gives a repo root or a general GitHub link.
   - Inspect the repo first, then decide whether to install directly with `--path` or tell the user about additional manual steps.

## Hard rules

- If the runtime prompt already shows the current Sesame skill catalog for this session, treat that list as the source of truth for "what is currently installed/loaded" unless the user explicitly asks you to rescan disk.
- When the user only asks which skills are currently available, answer from the prompt-provided Sesame catalog first instead of probing unrelated repo folders.
- **Always use the `sesame skill` CLI for Sesame skill installs.**
- **Only valid install targets are:**
  - global: `~/.sesame/skills`
  - workspace: `<workspace>/.sesame/skills`
- **The model should treat the two directories above as Sesame's only writable skill install roots.**
- **Never create or modify external platform directories when installing for Sesame**, including:
  - `.claude/`
  - `.codex/`
  - `.cursor/`
  - `.opencode/`
  - `.windsurf/`
  - `.qoder/`
  - `.kiro/`
  - `.gemini/`
  - `.agents/` root config files outside the Sesame install target
- If a repository contains multiple platform-specific skill layouts, treat `.claude/...` / `.codex/...` / template folders as **reference-only**, not Sesame install targets.
- A repository can still expose valid **source** skill directories such as:
  - `.sesame/skills/...`
  - `skills/...`
  - `.agents/skills/...`
  - `marketplace/skills/...`
  - repo root when `SKILL.md` exists there
- The chosen source is copied into Sesame's own directory. Do not mirror the repository's folder layout literally into external platform directories.

## Preferred commands

- List installed skills:
  - `sesame skill list`
  - `sesame skill list --scope global`
  - `sesame skill list --scope workspace`
- Inspect an ambiguous GitHub source before installing:
  - `sesame skill inspect https://github.com/<owner>/<repo>`
  - `sesame skill inspect <owner>/<repo>`
- Install a skill from a local directory:
  - `sesame skill install ./path/to/skill`
- Install a skill from GitHub when the path is already known:
  - `sesame skill install https://github.com/<owner>/<repo>/tree/<ref>/<path>`
  - `sesame skill install <owner>/<repo> --path <path/in/repo> --ref <ref>`
- Remove an installed skill:
  - `sesame skill remove <name>`
  - `sesame skill remove <name> --scope workspace`

## Decision rules

- If the link already targets a specific skill directory, install directly.
- If the link is a repo root or otherwise ambiguous, run `sesame skill inspect ...` first.
- If inspection reports multiple candidate source paths, ask the user which path to install or retry with `--path`.
- If inspection reports extra setup/configuration steps from the README, summarize those steps instead of pretending the skill can be installed directly.
- If inspection says a candidate path is platform-specific or ignored, do **not** bypass that by manually copying files into `.claude` or other non-Sesame folders.
- When talking about repo contents, call them **source paths** or **candidate source paths** to avoid confusing them with the final Sesame install directory.

## Scope rules

- Default install/remove scope is `global`, which maps to `~/.sesame/skills`.
- Workspace-local installs use `--scope workspace` and write to `<workspace>/.sesame/skills`.
- Bundled system skills live under `~/.sesame/skills/.system` and should not be removed manually.

## Notes

- In a source checkout, `sesame` might not be installed on `PATH`. If CLI access is required, use the repo-local launcher (for example `go run ./cmd/sesame ...`) from the repo root after verifying it exists, instead of assuming a globally installed binary.
- A skill install source must contain `SKILL.md`.
- After installing or removing a skill, Sesame should pick up the refreshed catalog on the next turn or the next explicit catalog view such as `/skills`.
