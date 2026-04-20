import { useEffect, useState } from "react";
import type { RoleSpec } from "../../api/types";

interface RoleEditorProps {
  role: RoleSpec | null;
  versions: RoleSpec[];
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
  policy: {},
  version: 1,
};

function parseSkills(input: string): string[] {
  return input
    .split(/[\n,]/g)
    .map((value) => value.trim())
    .filter(Boolean);
}

export function RoleEditor({
  role,
  versions,
  resetToken,
  isSaving,
  isDeleting,
  onSave,
  onDelete,
}: RoleEditorProps) {
  const [draft, setDraft] = useState<RoleSpec>(role ?? EMPTY_ROLE);
  const [skillsInput, setSkillsInput] = useState(role?.skills.join("\n") ?? "");
  const [policyInput, setPolicyInput] = useState(JSON.stringify(role?.policy ?? {}, null, 2));
  const [policyError, setPolicyError] = useState<string | null>(null);

  useEffect(() => {
    const next = role ?? EMPTY_ROLE;
    setDraft(next);
    setSkillsInput(next.skills.join("\n"));
    setPolicyInput(JSON.stringify(next.policy ?? {}, null, 2));
    setPolicyError(null);
  }, [role, resetToken]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let parsedPolicy: Record<string, unknown> = {};
    try {
      parsedPolicy = JSON.parse(policyInput || "{}") as Record<string, unknown>;
      setPolicyError(null);
    } catch {
      setPolicyError("Policy must be valid JSON.");
      return;
    }
    await onSave({
      ...draft,
      skills: parseSkills(skillsInput),
      policy: parsedPolicy,
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
        {policyError ? (
          <div
            className="mb-4 rounded-lg px-3 py-2 text-sm"
            role="alert"
            style={{
              backgroundColor: "rgba(220, 38, 38, 0.04)",
              border: "1px solid rgba(220, 38, 38, 0.18)",
              color: "var(--color-error)",
            }}
          >
            {policyError}
          </div>
        ) : null}

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
            Version
            <input
              aria-label="Version"
              value={String(draft.version || 1)}
              disabled
              className="mt-1 w-full rounded-md px-3 py-2"
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface-2)",
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

          <label className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            Policy
            <textarea
              aria-label="Policy"
              value={policyInput}
              onChange={(event) => {
                setPolicyInput(event.target.value);
                if (policyError) {
                  setPolicyError(null);
                }
              }}
              className="mt-1 w-full rounded-md px-3 py-2 font-mono text-sm"
              rows={8}
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text)",
                resize: "vertical",
              }}
            />
          </label>

          {isExisting && versions.length > 0 ? (
            <section
              className="rounded-lg border px-4 py-3"
              style={{
                backgroundColor: "var(--color-surface-2)",
                borderColor: "var(--color-border)",
              }}
            >
              <h2 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
                Version history
              </h2>
              <div className="mt-3 flex flex-col gap-2">
                {versions
                  .slice()
                  .sort((a, b) => b.version - a.version)
                  .map((version) => (
                    <div
                      key={`${version.role_id}:${version.version}`}
                      className="rounded-md px-3 py-2"
                      style={{
                        backgroundColor: "var(--color-surface)",
                        border: "1px solid var(--color-border)",
                      }}
                    >
                      <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
                        v{version.version} {version.display_name || version.role_id}
                      </div>
                      <div className="mt-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
                        Skills: {version.skills.join(", ") || "none"}
                      </div>
                    </div>
                  ))}
              </div>
            </section>
          ) : null}

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
