import { AlertTriangle } from "../Icon";

export function NoticeBlock({ text, count }: { text: string; count?: number }) {
  return (
    <div
      className="flex items-center gap-2 mb-1.5 px-3 py-1.5 rounded-md"
      style={{ backgroundColor: "var(--color-warning-dim)", border: "1px solid rgba(245,158,11,0.12)" }}
    >
      <AlertTriangle size={12} color="var(--color-warning)" className="shrink-0" />
      <span className="text-xs leading-relaxed" style={{ color: "var(--color-warning)" }}>
        {text}
      </span>
      {typeof count === "number" && count > 1 && (
        <span
          className="text-[10px] px-1.5 py-0.5 rounded font-medium"
          style={{ backgroundColor: "rgba(245,158,11,0.25)", color: "var(--color-warning)" }}
        >
          ×{count}
        </span>
      )}
    </div>
  );
}
