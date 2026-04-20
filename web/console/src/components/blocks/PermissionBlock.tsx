import { useSubmitPermission } from "../../api/queries";

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
  const submit = useSubmitPermission(""); // sessionId filled by parent context

  if (decision) {
    return (
      <div
        className="rounded-lg px-3 py-2 text-sm mb-2"
        style={{
          backgroundColor: "rgba(22,163,74,0.08)",
          border: "1px solid rgba(22,163,74,0.25)",
          color: "var(--color-success)",
        }}
      >
        ✓ {text}
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
      <div className="font-medium mb-1" style={{ color: "var(--color-accent)" }}>
        Permission Required
      </div>
      <div className="text-sm mb-2">{text}</div>
      <div className="flex gap-2">
        <button
          className="px-3 py-1 rounded text-sm"
          style={{
            backgroundColor: "var(--color-success)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
          }}
          onClick={() => submit.mutate({ requestId, decision: "approve" })}
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
          }}
          onClick={() => submit.mutate({ requestId, decision: "deny" })}
        >
          Deny
        </button>
      </div>
    </div>
  );
}
