import type { TokenUsage } from "../../api/types";

export function AssistantMessage({
  text,
  streaming,
  usage,
}: {
  text: string;
  streaming: boolean;
  usage?: TokenUsage;
}) {
  return (
    <div className="mb-4">
      <div className="flex items-baseline gap-3 mb-1">
        <span
          className="text-sm font-semibold"
          style={{ color: "var(--color-assistant)" }}
        >
          sesame
        </span>
        {streaming && (
          <span style={{ color: "var(--color-text-muted)" }}>·</span>
        )}
        {streaming && (
          <span
            className="text-xs animate-pulse"
            style={{ color: "var(--color-text-muted)" }}
          >
            streaming
          </span>
        )}
      </div>
      <div
        className="rounded-xl px-4 py-3 text-sm"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          color: "var(--color-text)",
          borderLeft: "3px solid var(--color-assistant)",
          lineHeight: 1.7,
          whiteSpace: "pre-wrap",
        }}
      >
        {text || (streaming ? "" : "(no response)")}
      </div>
      {usage && (
        <div
          className="flex gap-4 mt-1 text-xs"
          style={{ color: "var(--color-text-muted)" }}
        >
          <span>In: {usage.input_tokens.toLocaleString()}</span>
          <span>Out: {usage.output_tokens.toLocaleString()}</span>
          {usage.cached_tokens > 0 && (
            <span>Cache: {Math.round(usage.cache_hit_rate * 100)}%</span>
          )}
        </div>
      )}
    </div>
  );
}
