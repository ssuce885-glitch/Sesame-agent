package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go-agent/internal/types"
)

type metricsReader interface {
	GetMetricsOverview(context.Context, types.MetricsQuery) (types.MetricsOverviewResponse, error)
	ListMetricsTimeseries(context.Context, types.MetricsQuery) ([]types.MetricsTimeseriesPoint, error)
	ListMetricsTurns(context.Context, types.MetricsQuery) ([]types.TurnUsage, int, error)
}

func registerMetricsRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/metrics/overview", func(w http.ResponseWriter, r *http.Request) {
		handleGetMetricsOverview(deps)(w, r)
	})
	mux.HandleFunc("/v1/metrics/timeseries", func(w http.ResponseWriter, r *http.Request) {
		handleGetMetricsTimeseries(deps)(w, r)
	})
	mux.HandleFunc("/v1/metrics/turns", func(w http.ResponseWriter, r *http.Request) {
		handleGetMetricsTurns(deps)(w, r)
	})
}

func handleGetMetricsOverview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reader, query, ok := loadMetricsReaderAndQuery(w, r, deps)
		if !ok {
			return
		}

		overview, err := reader.GetMetricsOverview(r.Context(), query)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(overview)
	}
}

func handleGetMetricsTimeseries(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reader, query, ok := loadMetricsReaderAndQuery(w, r, deps)
		if !ok {
			return
		}

		points, err := reader.ListMetricsTimeseries(r.Context(), query)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.MetricsTimeseriesResponse{
			Bucket: normalizeMetricsBucketValue(query.Bucket),
			Points: points,
		})
	}
}

func handleGetMetricsTurns(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reader, query, ok := loadMetricsReaderAndQuery(w, r, deps)
		if !ok {
			return
		}

		items, total, err := reader.ListMetricsTurns(r.Context(), query)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		rows := make([]types.MetricsTurnRow, 0, len(items))
		titles := map[string]string{}
		for _, item := range items {
			title, ok := titles[item.SessionID]
			if !ok {
				title, _, err = deriveSessionText(r.Context(), deps, item.SessionID)
				if err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				titles[item.SessionID] = title
			}
			rows = append(rows, types.MetricsTurnRow{
				SessionID:    item.SessionID,
				SessionTitle: title,
				TurnID:       item.TurnID,
				Provider:     item.Provider,
				Model:        item.Model,
				InputTokens:  item.InputTokens,
				OutputTokens: item.OutputTokens,
				CachedTokens: item.CachedTokens,
				CacheHitRate: item.CacheHitRate,
				CreatedAt:    item.CreatedAt,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.MetricsTurnsResponse{
			Items:      rows,
			Page:       normalizedMetricsPage(query.Page),
			PageSize:   normalizedMetricsPageSize(query.PageSize),
			TotalCount: total,
		})
	}
}

func loadMetricsReaderAndQuery(w http.ResponseWriter, r *http.Request, deps Dependencies) (metricsReader, types.MetricsQuery, bool) {
	if deps.Store == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, types.MetricsQuery{}, false
	}
	reader, ok := deps.Store.(metricsReader)
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, types.MetricsQuery{}, false
	}

	query, err := parseMetricsQuery(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return nil, types.MetricsQuery{}, false
	}
	return reader, query, true
}

func parseMetricsQuery(r *http.Request) (types.MetricsQuery, error) {
	query := types.MetricsQuery{
		SessionID: r.URL.Query().Get("session_id"),
		Bucket:    normalizeMetricsBucketValue(r.URL.Query().Get("bucket")),
		Page:      normalizedMetricsPage(0),
		PageSize:  normalizedMetricsPageSize(0),
	}

	if raw := r.URL.Query().Get("from"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return types.MetricsQuery{}, err
		}
		query.From = value
		query.HasFrom = true
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return types.MetricsQuery{}, err
		}
		query.To = value
		query.HasTo = true
	}
	if raw := r.URL.Query().Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return types.MetricsQuery{}, err
		}
		query.Page = normalizedMetricsPage(value)
	}
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return types.MetricsQuery{}, err
		}
		query.PageSize = normalizedMetricsPageSize(value)
	}

	return query, nil
}

func normalizeMetricsBucketValue(bucket string) string {
	if bucket == "hour" {
		return "hour"
	}
	return "day"
}

func normalizedMetricsPage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizedMetricsPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 200 {
		return 200
	}
	return pageSize
}
