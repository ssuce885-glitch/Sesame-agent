import type { MetricsOverview } from "../../api/types";

export function SummaryCards({ metrics }: { metrics?: MetricsOverview }) {
  const total = metrics
    ? metrics.input_tokens + metrics.output_tokens
    : 0;

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
      value: metrics
        ? `${Math.round(metrics.cache_hit_rate * 100)}%`
        : "—",
      color:
        metrics && metrics.cache_hit_rate > 0.5
          ? "var(--color-success)"
          : "var(--color-text-muted)",
    },
  ];

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className="rounded-xl px-5 py-4"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
          }}
        >
          <div
            className="text-xs font-medium mb-3 uppercase tracking-wide"
            style={{ color: "var(--color-text-muted)" }}
          >
            {card.label}
          </div>
          <div
            className="text-2xl font-bold"
            style={{ color: card.color }}
          >
            {card.value}
          </div>
        </div>
      ))}
    </div>
  );
}
