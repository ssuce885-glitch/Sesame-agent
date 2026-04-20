import { useEffect, useState } from "react";
import type { RoleSpec } from "../../api/types";

interface RoleEditorProps {
  role: RoleSpec | null;
  resetToken: number;
  isSaving: boolean;
  isDeleting: boolean;
  onSave: (role: RoleSpec) => Promise<void> | void;
  onDelete: () => Promise<void> | void;
}

const EMPTY_ROLE: RoleSpec = {
  role_id: "",
  display_name: "",
  description: "",
  prompt: "",
  skills: [],
};

function parseSkills(input: string): string[] {
  return input
    .split(/[\n,]/g)
    .map((value) => value.trim())
    .filter(Boolean);
}

export function RoleEditor({
  role,
  resetToken,
  isSaving,
  isDeleting,
  onSave,
  onDelete,
}: RoleEditorProps) {
  const [draft, setDraft] = useState<RoleSpec>(role ?? EMPTY_ROLE);
  const [skillsInput, setSkillsInput] = useState(role?.skills.join("\n") ?? "");

  useEffect(() => {
    const next = role ?? EMPTY_ROLE;
    setDraft(next);
    setSkillsInput(next.skills.join("\n"));
  }, [role, resetToken]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await onSave({
      ...draft,
      skills: parseSkills(skillsInput),
    });
  }

  const isExisting = !!role;

  return (
    <section className="flex-1 overflow-y-auto p-6" style={{ backgroundColor: "var(--color-bg)" }}>
      <div
        className="max-w-3xl rounded-xl p-5"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
        }}
      >
        <h1 className="text-lg font-semibold m-0" style={{ color: "var(--color-text)" }}>
          {isExisting ? "Edit role" : "New role"}
        </h1>
        <p className="text-sm mt-1 mb-4" style={{ color: "var(--color-text-muted)" }}>
          Specialist role definition for this workspace.
        </p>

        <form className="flex flex-col gap-4" onSubmit={handleSubmit}>
          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Role ID
            <input
              aria-label="Role ID"
              value={draft.role_id}
              disabled={isExisting}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, role_id: event.target.value }))
              }
              className="mt-1 w-full rounded-md px-3 py-2"
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: isExisting ? "var(--color-surface-2)" : "var(--color-surface)",
                color: "var(--color-text)",
              }}
            />
          </label>

          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Display Name
            <input
              aria-label="Display Name"
              value={draft.display_name}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, display_name: event.target.value }))
              }
              className="mt-1 w-full rounded-md px-3 py-2"
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text)",
              }}
            />
          </label>

          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Description
            <input
              aria-label="Description"
              value={draft.description}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, description: event.target.value }))
              }
              className="mt-1 w-full rounded-md px-3 py-2"
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text)",
              }}
            />
          </label>

          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Prompt
            <textarea
              aria-label="Prompt"
              value={draft.prompt}
              onChange={(event) =>
                setDraft((prev) => ({ ...prev, prompt: event.target.value }))
              }
              className="mt-1 w-full rounded-md px-3 py-2"
              rows={7}
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text)",
                resize: "vertical",
              }}
            />
          </label>

          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Skills
            <textarea
              aria-label="Skills"
              value={skillsInput}
              onChange={(event) => setSkillsInput(event.target.value)}
              className="mt-1 w-full rounded-md px-3 py-2"
              rows={3}
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text)",
                resize: "vertical",
              }}
            />
          </label>

          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="px-3 py-2 rounded-md text-sm font-medium"
              style={{
                border: "none",
                backgroundColor: "var(--color-accent)",
                color: "#fff",
                cursor: "pointer",
              }}
            >
              {isSaving ? "Saving..." : "Save"}
            </button>
            {isExisting && (
              <button
                type="button"
                disabled={isDeleting}
                onClick={() => void onDelete()}
                className="px-3 py-2 rounded-md text-sm font-medium"
                style={{
                  border: "1px solid var(--color-error)",
                  backgroundColor: "transparent",
                  color: "var(--color-error)",
                  cursor: "pointer",
                }}
              >
                {isDeleting ? "Deleting..." : "Delete role"}
              </button>
            )}
          </div>
        </form>
      </div>
    </section>
  );
}
