export function UserMessage({ text }: { text: string }) {
  return (
    <div className="flex gap-3 mb-5">
      <div
        className="w-7 h-7 rounded-full flex items-center justify-center shrink-0 mt-0.5"
        style={{ backgroundColor: "var(--color-user-dim)" }}
      >
        <span
          className="text-xs font-bold"
          style={{ color: "var(--color-user)" }}
        >
          U
        </span>
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-xs font-semibold mb-1" style={{ color: "var(--color-user)" }}>
          You
        </div>
        <div
          className="text-sm leading-relaxed whitespace-pre-wrap"
          style={{ color: "var(--color-text)" }}
        >
          {text}
        </div>
      </div>
    </div>
  );
}
