import type { RoleDiagnostic } from "../../api/types";

interface RoleDiagnosticsProps {
  diagnostics: RoleDiagnostic[];
}

export function RoleDiagnostics({ diagnostics }: RoleDiagnosticsProps) {
  if (diagnostics.length === 0) {
    return null;
  }

  return (
    <section
      className="shrink-0 border-b px-4 py-4 md:px-6"
      style={{
        backgroundColor: "rgba(220, 38, 38, 0.04)",
        borderColor: "rgba(220, 38, 38, 0.18)",
      }}
    >
      <div className="max-w-3xl">
        <h2 className="text-sm font-semibold m-0" style={{ color: "var(--color-error)" }}>
          Role diagnostics
        </h2>
        <p className="mt-1 text-sm" style={{ color: "var(--color-text-muted)" }}>
          Some role folders are incomplete and were skipped from the editable role list.
        </p>

        <div className="mt-3 flex flex-col gap-2">
          {diagnostics.map((diagnostic) => (
            <article
              key={diagnostic.role_id}
              className="rounded-lg border px-3 py-2"
              style={{
                backgroundColor: "var(--color-surface)",
                borderColor: "rgba(220, 38, 38, 0.18)",
              }}
            >
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
                  {diagnostic.role_id}
                </span>
                <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
                  {diagnostic.path}
                </span>
              </div>
              <p className="mt-1 text-sm" style={{ color: "var(--color-error)" }}>
                {diagnostic.error}
              </p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
