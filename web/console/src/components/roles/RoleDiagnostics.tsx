import type { RoleDiagnostic } from "../../api/types";
import { AlertTriangle, X } from "../Icon";

interface RoleDiagnosticsProps {
  diagnostics: RoleDiagnostic[];
}

export function RoleDiagnostics({ diagnostics }: RoleDiagnosticsProps) {
  if (diagnostics.length === 0) return null;

  return (
    <div
      className="flex items-start gap-2 px-4 py-2.5"
      style={{
        backgroundColor: "var(--color-error-dim)",
        borderBottom: "1px solid rgba(239,68,68,0.1)",
      }}
    >
      <AlertTriangle size={14} color="var(--color-error)" className="shrink-0 mt-0.5" />
      <div className="flex-1 min-w-0">
        <div className="text-xs font-medium" style={{ color: "var(--color-error)" }}>
          Role diagnostics
        </div>
        <div className="mt-1 space-y-1">
          {diagnostics.map((d) => (
            <div key={d.role_id} className="flex items-center gap-1.5 text-xs">
              <span className="font-medium" style={{ color: "var(--color-text)" }}>{d.role_id}</span>
              <span style={{ color: "var(--color-text-tertiary)" }}>{d.path}</span>
              <span style={{ color: "var(--color-error)" }}>{d.error}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
