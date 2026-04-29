import { useEffect, useState } from "react";
import type { RoleSpec } from "../../api/types";
import { MarkdownRenderer } from "../MarkdownRenderer";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { vscDarkPlus } from "react-syntax-highlighter/dist/esm/styles/prism";
import { Save, Trash, Plus, X, Eye, EyeOff } from "../Icon";

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

type Tab = "basic" | "prompt" | "skills" | "policy" | "versions";

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
  const [activeTab, setActiveTab] = useState<Tab>("basic");
  const [showPromptPreview, setShowPromptPreview] = useState(false);

  useEffect(() => {
    const next = role ?? EMPTY_ROLE;
    setDraft(next);
    setSkillsInput(next.skills.join("\n"));
    setPolicyInput(JSON.stringify(next.policy ?? {}, null, 2));
    setPolicyError(null);
    setActiveTab("basic");
  }, [role, resetToken]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let parsedPolicy: Record<string, unknown> = {};
    try {
      parsedPolicy = JSON.parse(policyInput || "{}") as Record<string, unknown>;
      setPolicyError(null);
    } catch {
      setPolicyError("Policy must be valid JSON.");
      setActiveTab("policy");
      return;
    }
    await onSave({
      ...draft,
      skills: parseSkills(skillsInput),
      policy: parsedPolicy,
    });
  }

  const isExisting = !!role;
  const tabs: { key: Tab; label: string }[] = [
    { key: "basic", label: "Basic" },
    { key: "prompt", label: "Prompt" },
    { key: "skills", label: "Skills" },
    { key: "policy", label: "Policy" },
  ];
  if (isExisting && versions.length > 0) {
    tabs.push({ key: "versions", label: "Versions" });
  }

  return (
    <section className="flex flex-col h-full overflow-hidden" style={{ backgroundColor: "var(--color-bg)" }}>
      <form className="flex flex-col h-full" onSubmit={handleSubmit}>
        {/* Header */}
        <div
          className="flex items-center justify-between px-5 py-3 shrink-0"
          style={{ borderBottom: "1px solid var(--color-border)" }}
        >
          <div className="flex items-center gap-3">
            <h1 className="text-base font-semibold m-0" style={{ color: "var(--color-text)" }}>
              {isExisting ? "Edit role" : "New role"}
            </h1>
            {isExisting && (
              <span className="text-[11px] px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}>
                v{draft.version}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: "var(--color-accent)",
                color: "#fff",
                border: "none",
                cursor: isSaving ? "not-allowed" : "pointer",
                opacity: isSaving ? 0.7 : 1,
              }}
            >
              <Save size={13} />
              {isSaving ? "Saving..." : "Save"}
            </button>
            {isExisting && (
              <button
                type="button"
                disabled={isDeleting}
                onClick={() => void onDelete()}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: "transparent",
                  border: "1px solid var(--color-error)",
                  color: "var(--color-error)",
                  cursor: isDeleting ? "not-allowed" : "pointer",
                  opacity: isDeleting ? 0.7 : 1,
                }}
              >
                <Trash size={13} />
                {isDeleting ? "Deleting..." : "Delete"}
              </button>
            )}
          </div>
        </div>

        {/* Tabs */}
        <div className="flex items-center gap-0 px-5 shrink-0" style={{ borderBottom: "1px solid var(--color-border)" }}>
          {tabs.map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => setActiveTab(tab.key)}
              className="text-xs font-medium px-3 py-2"
              style={{
                backgroundColor: "transparent",
                border: "none",
                borderBottom: activeTab === tab.key ? "2px solid var(--color-accent)" : "2px solid transparent",
                color: activeTab === tab.key ? "var(--color-accent)" : "var(--color-text-tertiary)",
                cursor: "pointer",
                transition: "color 0.15s",
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Error banner */}
        {policyError && (
          <div
            className="flex items-center gap-2 px-5 py-2 text-xs"
            style={{ backgroundColor: "var(--color-error-dim)", color: "var(--color-error)" }}
            role="alert"
          >
            <X size={12} />
            {policyError}
          </div>
        )}

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {activeTab === "basic" && (
            <div className="flex flex-col gap-4 max-w-xl">
              <Field label="Role ID">
                <input
                  value={draft.role_id}
                  disabled={isExisting}
                  onChange={(e) => setDraft((prev) => ({ ...prev, role_id: e.target.value }))}
                  className="w-full rounded-md px-3 py-2 text-sm outline-none"
                  style={{
                    border: "1px solid var(--color-border)",
                    backgroundColor: isExisting ? "var(--color-surface)" : "var(--color-bg-elevated)",
                    color: "var(--color-text)",
                  }}
                />
              </Field>
              <Field label="Display Name">
                <input
                  value={draft.display_name}
                  onChange={(e) => setDraft((prev) => ({ ...prev, display_name: e.target.value }))}
                  className="w-full rounded-md px-3 py-2 text-sm outline-none"
                  style={{
                    border: "1px solid var(--color-border)",
                    backgroundColor: "var(--color-bg-elevated)",
                    color: "var(--color-text)",
                  }}
                />
              </Field>
              <Field label="Description">
                <input
                  value={draft.description}
                  onChange={(e) => setDraft((prev) => ({ ...prev, description: e.target.value }))}
                  className="w-full rounded-md px-3 py-2 text-sm outline-none"
                  style={{
                    border: "1px solid var(--color-border)",
                    backgroundColor: "var(--color-bg-elevated)",
                    color: "var(--color-text)",
                  }}
                />
              </Field>
            </div>
          )}

          {activeTab === "prompt" && (
            <div className="flex flex-col gap-3 h-full">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium" style={{ color: "var(--color-text-tertiary)" }}>
                  Prompt supports Markdown
                </span>
                <button
                  type="button"
                  onClick={() => setShowPromptPreview((v) => !v)}
                  className="flex items-center gap-1 text-[11px] rounded px-2 py-1"
                  style={{
                    backgroundColor: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    color: "var(--color-text-secondary)",
                    cursor: "pointer",
                  }}
                >
                  {showPromptPreview ? <EyeOff size={12} /> : <Eye size={12} />}
                  {showPromptPreview ? "Hide preview" : "Preview"}
                </button>
              </div>
              <div className={showPromptPreview ? "grid grid-cols-2 gap-3 flex-1" : "flex-1"}>
                <textarea
                  value={draft.prompt}
                  onChange={(e) => setDraft((prev) => ({ ...prev, prompt: e.target.value }))}
                  className="w-full h-full rounded-md px-3 py-2 text-sm outline-none resize-none font-mono"
                  style={{
                    border: "1px solid var(--color-border)",
                    backgroundColor: "var(--color-bg-elevated)",
                    color: "var(--color-text)",
                    lineHeight: 1.6,
                    minHeight: 300,
                  }}
                  spellCheck={false}
                />
                {showPromptPreview && (
                  <div
                    className="rounded-md px-3 py-2 overflow-y-auto"
                    style={{
                      border: "1px solid var(--color-border)",
                      backgroundColor: "var(--color-bg-elevated)",
                      minHeight: 300,
                    }}
                  >
                    <MarkdownRenderer content={draft.prompt} />
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === "skills" && (
            <div className="flex flex-col gap-3 max-w-xl">
              <p className="text-xs m-0" style={{ color: "var(--color-text-tertiary)" }}>
                Enter one skill per line or comma-separated.
              </p>
              <textarea
                value={skillsInput}
                onChange={(e) => setSkillsInput(e.target.value)}
                className="w-full rounded-md px-3 py-2 text-sm outline-none resize-none font-mono"
                rows={6}
                style={{
                  border: "1px solid var(--color-border)",
                  backgroundColor: "var(--color-bg-elevated)",
                  color: "var(--color-text)",
                  lineHeight: 1.6,
                }}
              />
              <div className="flex flex-wrap gap-1.5">
                {draft.skills.map((s) => (
                  <span
                    key={s}
                    className="flex items-center gap-1 text-[11px] rounded px-2 py-1"
                    style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)", color: "var(--color-text-secondary)" }}
                  >
                    {s}
                    <button
                      type="button"
                      onClick={() => {
                        const next = draft.skills.filter((sk) => sk !== s);
                        setDraft((prev) => ({ ...prev, skills: next }));
                        setSkillsInput(next.join("\n"));
                      }}
                      className="flex items-center justify-center"
                      style={{ background: "transparent", border: "none", color: "var(--color-text-tertiary)", cursor: "pointer", padding: 0 }}
                    >
                      <X size={10} />
                    </button>
                  </span>
                ))}
              </div>
            </div>
          )}

          {activeTab === "policy" && (
            <div className="flex flex-col gap-3 h-full">
              <textarea
                value={policyInput}
                onChange={(e) => {
                  setPolicyInput(e.target.value);
                  if (policyError) setPolicyError(null);
                }}
                className="w-full h-full rounded-md px-3 py-2 text-sm outline-none resize-none font-mono"
                style={{
                  border: `1px solid ${policyError ? "var(--color-error)" : "var(--color-border)"}`,
                  backgroundColor: "var(--color-bg-elevated)",
                  color: "var(--color-text)",
                  lineHeight: 1.6,
                  minHeight: 300,
                }}
                spellCheck={false}
              />
            </div>
          )}

          {activeTab === "versions" && (
            <div className="flex flex-col gap-2 max-w-xl">
              {versions
                .slice()
                .sort((a, b) => b.version - a.version)
                .map((version) => (
                  <div
                    key={`${version.role_id}:${version.version}`}
                    className="rounded-md px-3 py-2.5"
                    style={{
                      backgroundColor: "var(--color-bg-elevated)",
                      border: "1px solid var(--color-border)",
                    }}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
                        v{version.version} {version.display_name || version.role_id}
                      </span>
                      <span className="text-[10px] px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}>
                        {version.skills.length} skills
                      </span>
                    </div>
                    <div className="text-xs mt-1" style={{ color: "var(--color-text-tertiary)" }}>
                      {version.skills.join(", ") || "no skills"}
                    </div>
                  </div>
                ))}
            </div>
          )}
        </div>
      </form>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium" style={{ color: "var(--color-text-secondary)" }}>
        {label}
      </span>
      {children}
    </label>
  );
}
