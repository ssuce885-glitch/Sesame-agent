import { useMetricsOverview, useMetricsTimeseries } from "../api/queries";
import { SummaryCards } from "../components/usage/SummaryCards";
import { UsageChart } from "../components/usage/UsageChart";
import { useI18n } from "../i18n";
import { BarChart } from "../components/Icon";

interface UsagePageProps {
  sessionId?: string;
}

export function UsagePage({ sessionId }: UsagePageProps) {
  const { t } = useI18n();
  const { data: overview, isLoading: loadingOverview } = useMetricsOverview(sessionId);
  const { data: timeseries, isLoading: loadingTimeseries } = useMetricsTimeseries(sessionId);

  return (
    <div className="flex flex-col gap-4 overflow-y-auto p-4 md:p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="flex items-center gap-2">
        <BarChart size={18} color="var(--color-text-tertiary)" />
        <div>
          <h1 className="text-lg font-bold m-0" style={{ color: "var(--color-text)" }}>
            {t("usage.title")}
          </h1>
          <p className="text-xs m-0 mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {sessionId ? t("usage.currentSession") : t("usage.allSessions")} — {t("usage.last30Days")}
          </p>
        </div>
      </div>

      {loadingOverview ? (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <div
              key={i}
              className="rounded-lg px-4 py-3 animate-shimmer"
              style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)", height: 72 }}
            />
          ))}
        </div>
      ) : (
        <SummaryCards metrics={overview} />
      )}

      {loadingTimeseries ? (
        <div
          className="rounded-lg animate-shimmer"
          style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)", height: 300 }}
        />
      ) : (
        <UsageChart data={timeseries} />
      )}
    </div>
  );
}
