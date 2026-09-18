package store

import (
	"context"
	"time"
)

// FeedbackFilter selects feedback-kind ledger events across tickets.
type FeedbackFilter struct {
	Since  *time.Time // ts >= Since
	Source string    // payload->>'source' exact match; empty = any
	Limit  int
}

// Feedback lists feedback-kind events across all tickets, newest first.
// Backed by a direct ledger scan on kind. Measure before adding a
// projection/index (same policy as Analytics).
func (s *Store) Feedback(ctx context.Context, f FeedbackFilter) ([]LedgerEvent, error) {
	if f.Limit <= 0 || f.Limit > 10000 {
		f.Limit = 1000
	}
	rows, err := s.Pool.Query(ctx, `
SELECT id, ulid, ticket_ulid, ts, actor, actor_type::text, session, kind, payload
FROM ledger
WHERE kind='feedback'
  AND ($1::timestamptz IS NULL OR ts >= $1)
  AND ($2::text IS NULL OR payload->>'source' = $2)
ORDER BY id DESC
LIMIT $3`,
		f.Since, nullableString(f.Source), f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	evs := []LedgerEvent{}
	for rows.Next() {
		var e LedgerEvent
		if err := rows.Scan(&e.ID, &e.ULID, &e.Ticket, &e.TS, &e.Actor, &e.ActorType, &e.Session, &e.Kind, &e.Payload); err != nil {
			return nil, err
		}
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
