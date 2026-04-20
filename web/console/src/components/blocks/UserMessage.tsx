export function UserMessage({ text }: { text: string }) {
  return (
    <div className="mb-4">
      <div className="flex items-baseline gap-3 mb-1">
        <span
          className="text-sm font-semibold"
          style={{ color: "var(--color-user)" }}
        >
          you
        </span>
      </div>
      <div
        className="rounded-xl px-4 py-3 text-sm"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          color: "var(--color-text)",
          borderLeft: "3px solid var(--color-user)",
          lineHeight: 1.6,
          whiteSpace: "pre-wrap",
        }}
      >
        {text}
      </div>
    </div>
  );
}
