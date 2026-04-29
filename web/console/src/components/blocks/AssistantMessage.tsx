import { MarkdownRenderer } from "../MarkdownRenderer";
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
    <div className="flex gap-3 mb-5">
      <div
        className="w-7 h-7 rounded-full flex items-center justify-center shrink-0 mt-0.5"
        style={{ backgroundColor: "var(--color-assistant-dim)" }}
      >
        <span
          className="text-xs font-bold"
          style={{ color: "var(--color-assistant)" }}
        >
          S
        </span>
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-xs font-semibold" style={{ color: "var(--color-assistant)" }}>
            Sesame
          </span>
          {streaming && (
            <span
              className="inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded-full"
              style={{
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text-tertiary)",
                border: "1px solid var(--color-border)",
              }}
            >
              <span
                className="w-1.5 h-1.5 rounded-full animate-pulse-amber"
                style={{ backgroundColor: "var(--color-running)" }}
              />
              streaming
            </span>
          )}
        </div>

        <div className="text-sm leading-relaxed" style={{ color: "var(--color-text)" }}>
          {text || streaming ? (
            <MarkdownRenderer content={text} />
          ) : (
            <span style={{ color: "var(--color-text-tertiary)" }}>(no response)</span>
          )}
        </div>

        {usage && (
          <div
            className="flex items-center gap-3 mt-2 text-[11px]"
            style={{ color: "var(--color-text-tertiary)" }}
          >
            <span>In: {usage.input_tokens.toLocaleString()}</span>
            <span>Out: {usage.output_tokens.toLocaleString()}</span>
            {usage.cached_tokens > 0 && (
              <span
                className="px-1 rounded"
                style={{ backgroundColor: "var(--color-accent-dim)", color: "var(--color-accent)" }}
              >
                Cache: {Math.round(usage.cache_hit_rate * 100)}%
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
