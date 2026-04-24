import { AlertTriangle } from "../Icon";

export function NoticeBlock({ text }: { text: string }) {
  return (
    <div
      className="rounded-lg px-3 py-2 text-sm mb-2 flex items-start gap-2"
      style={{
        backgroundColor: "rgba(217,119,6,0.08)",
        border: "1px solid rgba(217,119,6,0.25)",
        color: "var(--color-warning)",
      }}
    >
      <AlertTriangle size={14} color="var(--color-warning)" className="mt-0.5 shrink-0" />
      <span>{text}</span>
    </div>
  );
}
