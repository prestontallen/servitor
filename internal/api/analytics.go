package api

import (
	"context"
	"time"
)

// DayBucket is one day of ledger activity (the time-series read).
type DayBucket struct {
	Day    string           `json:"day"`
	Events int64            `json:"events"`
	ByKind map[string]int64 `json:"by_kind"`
}

// Analytics returns per-day ledger activity for the last N days.
//
// Backed by a direct GROUP BY over the ledger for now. The continuous
// aggregates (cagg_events_per_day / cagg_events_kind_day, refreshed hourly
// by policy) answer the same query shape sub-millisecond; the switch is a
// one-line change with zero downstream impact because THIS shape is the
// contract. Measure first (see the benchmark in the spike), swap when the
// corpus justifies it.
func (ss *StoreService) Analytics(ctx context.Context, days int) ([]DayBucket, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := ss.Store.Pool.Query(ctx, `
SELECT date_trunc('day', ts)::date::text AS day, kind, count(*)
FROM ledger WHERE ts >= $1 GROUP BY 1, 2 ORDER BY 1`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]int64{}
	byKind := map[string]map[string]int64{}
	var order []string
	for rows.Next() {
		var d, k string
		var n int64
		if err := rows.Scan(&d, &k, &n); err != nil {
			return nil, err
		}
		if _, seen := byDay[d]; !seen {
			order = append(order, d)
		}
		byDay[d] += n
		if byKind[d] == nil {
			byKind[d] = map[string]int64{}
		}
		byKind[d][k] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]DayBucket, 0, len(order))
	for _, d := range order {
		out = append(out, DayBucket{Day: d, Events: byDay[d], ByKind: byKind[d]})
	}
	return out, nil
}
