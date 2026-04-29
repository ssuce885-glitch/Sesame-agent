import type { MetricsOverview } from "../../api/types";

export function SummaryCards({ metrics }: { metrics?: MetricsOverview }) {
  const total = metrics ? metrics.input_tokens + metrics.output_tokens : 0;

  const cards = [
    {
      label: "Input Tokens",
      value: metrics?.input_tokens.toLocaleString() ?? "—",
      color: "var(--color-accent)",
    },
    {
      label: "Output Tokens",
      value: metrics?.output_tokens.toLocaleString() ?? "—",
      color: "var(--color-assistant)",
    },
    {
      label: "Total",
      value: total > 0 ? total.toLocaleString() : "—",
      color: "var(--color-text)",
    },
    {
      label: "Cache Hit",
      value: metrics ? `${Math.round(metrics.cache_hit_rate * 100)}%` : "—",
      color: metrics && metrics.cache_hit_rate > 0.5 ? "var(--color-success)" : "var(--color-text-tertiary)",
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className="rounded-lg px-4 py-3 relative overflow-hidden"
          style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
        >
          <div className="absolute bottom-0 left-0 right-0 h-0.5" style={{ backgroundColor: card.color }} />
          <div className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--color-text-tertiary)" }}>
            {card.label}
          </div>
          <div className="mt-1 text-xl font-bold tabular-nums" style={{ color: card.color }}>
            {card.value}
          </div>
        </div>
      ))}
    </div>
  );
}
