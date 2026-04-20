export function ErrorBlock({ text }: { text: string }) {
  return (
    <div
      className="rounded-lg px-3 py-2 text-sm mb-2"
      style={{
        backgroundColor: "rgba(220,38,38,0.08)",
        border: "1px solid rgba(220,38,38,0.25)",
        color: "var(--color-error)",
      }}
    >
      ✗ {text}
    </div>
  );
}
