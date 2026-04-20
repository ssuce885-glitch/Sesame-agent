import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import type { MetricsTimeseries } from "../../api/types";

export function UsageChart({ data }: { data?: MetricsTimeseries }) {
  if (!data || data.points.length === 0) {
    return (
      <div
        className="rounded-xl px-5 py-8 text-center text-sm"
        style={{
          backgroundColor: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          color: "var(--color-text-muted)",
        }}
      >
        No usage data yet.
      </div>
    );
  }

  const chartData = data.points.map((p) => ({
    date: new Date(p.bucket_start).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
    }),
    input: p.input_tokens,
    output: p.output_tokens,
    cached: p.cached_tokens,
  }));

  return (
    <div
      className="rounded-xl px-5 py-4"
      style={{
        backgroundColor: "var(--color-surface)",
        border: "1px solid var(--color-border)",
      }}
    >
      <div className="text-sm font-semibold mb-4" style={{ color: "var(--color-text)" }}>
        Daily Token Usage
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <AreaChart data={chartData} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 11, fill: "var(--color-text-muted)" }}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            tick={{ fontSize: 11, fill: "var(--color-text-muted)" }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v: number) =>
              String(v >= 1000 ? `${(v / 1000).toFixed(0)}k` : v)
            }
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 8,
              fontSize: 12,
              color: "var(--color-text)",
            }}
            labelStyle={{ fontWeight: 600 }}
          />
          <Legend
            wrapperStyle={{ fontSize: 12, color: "var(--color-text-muted)" }}
          />
          <Area
            type="monotone"
            dataKey="input"
            stackId="1"
            stroke="var(--color-accent)"
            fill="var(--color-accent)"
            fillOpacity={0.7}
            name="Input"
          />
          <Area
            type="monotone"
            dataKey="output"
            stackId="2"
            stroke="var(--color-assistant)"
            fill="var(--color-assistant)"
            fillOpacity={0.7}
            name="Output"
          />
          <Area
            type="monotone"
            dataKey="cached"
            stackId="3"
            stroke="var(--color-text-muted)"
            fill="var(--color-text-muted)"
            fillOpacity={0.3}
            name="Cached"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
