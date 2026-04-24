import { useSubmitPermission } from "../../api/queries";
import { Shield, Check } from "../Icon";

interface PermissionBlockProps {
  requestId: string;
  profile: string;
  reason?: string;
  decision?: string;
  text: string;
}

export function PermissionBlock({
  requestId,
  profile,
  reason,
  decision,
  text,
}: PermissionBlockProps) {
  const submit = useSubmitPermission("");

  if (decision) {
    return (
      <div
        className="rounded-lg px-3 py-2 text-sm mb-2 flex items-start gap-2"
        style={{
          backgroundColor: "rgba(22,163,74,0.08)",
          border: "1px solid rgba(22,163,74,0.25)",
          color: "var(--color-success)",
        }}
      >
        <Check size={14} color="var(--color-success)" className="mt-0.5 shrink-0" />
        <span>{text}</span>
      </div>
    );
  }

  return (
    <div
      className="rounded-lg px-3 py-3 text-sm mb-2"
      style={{
        backgroundColor: "rgba(8,145,178,0.06)",
        border: "1px solid rgba(8,145,178,0.2)",
        color: "var(--color-text)",
      }}
    >
      <div className="font-medium mb-1 flex items-center gap-1.5" style={{ color: "var(--color-accent)" }}>
        <Shield size={14} color="var(--color-accent)" />
        Permission Required
      </div>
      <div className="text-sm mb-2">{text}</div>
      <div className="flex gap-2">
        <button
          className="px-3 py-1 rounded text-sm flex items-center gap-1"
          style={{
            backgroundColor: "var(--color-success)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
            transition: "opacity 0.15s",
          }}
          onClick={() => submit.mutate({ requestId, decision: "approve" })}
          onMouseEnter={(e) => { e.currentTarget.style.opacity = "0.85"; }}
          onMouseLeave={(e) => { e.currentTarget.style.opacity = "1"; }}
        >
          Approve
        </button>
        <button
          className="px-3 py-1 rounded text-sm"
          style={{
            backgroundColor: "var(--color-surface-2)",
            color: "var(--color-text)",
            border: "1px solid var(--color-border)",
            cursor: "pointer",
            transition: "border-color 0.15s",
          }}
          onClick={() => submit.mutate({ requestId, decision: "deny" })}
          onMouseEnter={(e) => { e.currentTarget.style.borderColor = "var(--color-text-muted)"; }}
          onMouseLeave={(e) => { e.currentTarget.style.borderColor = "var(--color-border)"; }}
        >
          Deny
        </button>
      </div>
    </div>
  );
}
