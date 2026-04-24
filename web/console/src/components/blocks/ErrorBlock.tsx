import { X } from "../Icon";

export function ErrorBlock({ text }: { text: string }) {
  return (
    <div
      className="rounded-lg px-3 py-2 text-sm mb-2 flex items-start gap-2"
      style={{
        backgroundColor: "rgba(220,38,38,0.08)",
        border: "1px solid rgba(220,38,38,0.25)",
        color: "var(--color-error)",
      }}
    >
      <X size={14} color="var(--color-error)" className="mt-0.5 shrink-0" />
      <span>{text}</span>
    </div>
  );
}
