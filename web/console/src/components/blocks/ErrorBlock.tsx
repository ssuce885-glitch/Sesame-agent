import { X } from "../Icon";

export function ErrorBlock({ text, count }: { text: string; count?: number }) {
  return (
    <div
      className="flex items-center gap-2 mb-1.5 px-3 py-1.5 rounded-md"
      style={{ backgroundColor: "var(--color-error-dim)", border: "1px solid rgba(239,68,68,0.12)" }}
    >
      <X size={12} color="var(--color-error)" className="shrink-0" />
      <span className="text-xs leading-relaxed" style={{ color: "var(--color-error)" }}>
        {text}
      </span>
      {typeof count === "number" && count > 1 && (
        <span
          className="text-[10px] px-1.5 py-0.5 rounded font-medium"
          style={{ backgroundColor: "rgba(239,68,68,0.25)", color: "var(--color-error)" }}
        >
          ×{count}
        </span>
      )}
    </div>
  );
}
