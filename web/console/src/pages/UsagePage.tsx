import { useMetricsOverview, useMetricsTimeseries } from "../api/queries";
import { SummaryCards } from "../components/usage/SummaryCards";
import { UsageChart } from "../components/usage/UsageChart";
import { useI18n } from "../i18n";

interface UsagePageProps {
  sessionId?: string;
}

export function UsagePage({ sessionId }: UsagePageProps) {
  const { t } = useI18n();
  const { data: overview, isLoading: loadingOverview } = useMetricsOverview(sessionId);
  const { data: timeseries, isLoading: loadingTimeseries } = useMetricsTimeseries(sessionId);

  return (
    <div className="flex flex-col gap-6 overflow-y-auto p-4 md:p-6" style={{ backgroundColor: "var(--color-bg)" }}>
      {/* Page title */}
      <div>
        <h1
          className="text-lg font-semibold"
          style={{ color: "var(--color-text)" }}
        >
          {t("usage.title")}
        </h1>
        <p className="text-sm mt-0.5" style={{ color: "var(--color-text-muted)" }}>
          {sessionId ? t("usage.currentSession") : t("usage.allSessions")} — {t("usage.last30Days")}
        </p>
      </div>

      {/* Summary cards */}
      {loadingOverview ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <div
              key={i}
              className="rounded-xl px-5 py-4 animate-pulse"
              style={{
                backgroundColor: "var(--color-surface)",
                border: "1px solid var(--color-border)",
              }}
            >
              <div
                className="h-3 w-20 rounded mb-3"
                style={{ backgroundColor: "var(--color-border)" }}
              />
              <div
                className="h-8 w-24 rounded"
                style={{ backgroundColor: "var(--color-border)" }}
              />
            </div>
          ))}
        </div>
      ) : (
        <SummaryCards metrics={overview} />
      )}

      {/* Timeseries chart */}
      {loadingTimeseries ? (
        <div
          className="rounded-xl px-5 py-4 animate-pulse"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            height: 300,
          }}
        />
      ) : (
        <UsageChart data={timeseries} />
      )}
    </div>
  );
}
