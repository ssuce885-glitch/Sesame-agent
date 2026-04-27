package sqlite

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) GetMetricsOverview(ctx context.Context, query types.MetricsQuery) (types.MetricsOverviewResponse, error) {
	items, err := s.listTurnUsageRows(ctx, query, false)
	if err != nil {
		return types.MetricsOverviewResponse{}, err
	}

	var overview types.MetricsOverviewResponse
	for _, item := range items {
		overview.InputTokens += item.InputTokens
		overview.OutputTokens += item.OutputTokens
		overview.CachedTokens += item.CachedTokens
	}
	if overview.InputTokens > 0 {
		overview.CacheHitRate = float64(overview.CachedTokens) / float64(overview.InputTokens)
	}

	return overview, nil
}

func (s *Store) ListMetricsTimeseries(ctx context.Context, query types.MetricsQuery) ([]types.MetricsTimeseriesPoint, error) {
	items, err := s.listTurnUsageRows(ctx, query, false)
	if err != nil {
		return nil, err
	}

	bucket := normalizeMetricsBucket(query.Bucket)
	orderedKeys := make([]string, 0)
	byKey := map[string]*types.MetricsTimeseriesPoint{}
	for _, item := range items {
		bucketStart := truncateMetricsTime(item.CreatedAt, bucket)
		key := bucketStart.Format(time.RFC3339)
		point, ok := byKey[key]
		if !ok {
			point = &types.MetricsTimeseriesPoint{BucketStart: bucketStart}
			byKey[key] = point
			orderedKeys = append(orderedKeys, key)
		}
		point.InputTokens += item.InputTokens
		point.OutputTokens += item.OutputTokens
		point.CachedTokens += item.CachedTokens
	}

	points := make([]types.MetricsTimeseriesPoint, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		points = append(points, *byKey[key])
	}
	return points, nil
}

func (s *Store) ListMetricsTurns(ctx context.Context, query types.MetricsQuery) ([]types.TurnUsage, int, error) {
	total, err := s.countTurnUsageRows(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.listTurnUsageRows(ctx, query, true)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Store) countTurnUsageRows(ctx context.Context, query types.MetricsQuery) (int, error) {
	sqlText, args := buildTurnUsageBaseQuery(`
		select count(*)
		from turn_usage
	`, query)
	var total int
	err := s.db.QueryRowContext(ctx, sqlText, args...).Scan(&total)
	return total, err
}

func (s *Store) listTurnUsageRows(ctx context.Context, query types.MetricsQuery, paginate bool) ([]types.TurnUsage, error) {
	sqlText, args := buildTurnUsageBaseQuery(`
		select turn_id, session_id, provider, model, input_tokens, output_tokens, cached_tokens, cache_hit_rate, created_at, updated_at
		from turn_usage
	`, query)
	sqlText += " order by created_at desc, turn_id desc"
	if paginate {
		page := query.Page
		if page <= 0 {
			page = 1
		}
		pageSize := query.PageSize
		if pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 200 {
			pageSize = 200
		}
		sqlText += " limit ? offset ?"
		args = append(args, pageSize, (page-1)*pageSize)
	}

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.TurnUsage, 0)
	for rows.Next() {
		var item types.TurnUsage
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&item.TurnID,
			&item.SessionID,
			&item.Provider,
			&item.Model,
			&item.InputTokens,
			&item.OutputTokens,
			&item.CachedTokens,
			&item.CacheHitRate,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		item.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func buildTurnUsageBaseQuery(prefix string, query types.MetricsQuery) (string, []any) {
	parts := []string{strings.TrimSpace(prefix), "where 1 = 1"}
	args := make([]any, 0, 4)

	if query.SessionID != "" {
		parts = append(parts, "and session_id = ?")
		args = append(args, query.SessionID)
	}
	if query.HasFrom {
		parts = append(parts, "and created_at >= ?")
		args = append(args, query.From.UTC().Format(timeLayout))
	}
	if query.HasTo {
		parts = append(parts, "and created_at <= ?")
		args = append(args, query.To.UTC().Format(timeLayout))
	}

	return strings.Join(parts, "\n"), args
}

func normalizeMetricsBucket(bucket string) string {
	switch bucket {
	case "hour":
		return "hour"
	default:
		return "day"
	}
}

func truncateMetricsTime(ts time.Time, bucket string) time.Time {
	ts = ts.UTC()
	if bucket == "hour" {
		return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), 0, 0, 0, time.UTC)
	}
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC)
}
