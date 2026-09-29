package store

import (
	"context"
	"time"
)

// FeedbackFilter selects feedback-kind ledger events across tickets.
type FeedbackFilter struct {
	Since  *time.Time // ts >= Since
	Source string     // payload->>'source' exact match; empty = any
	Limit  int
}

// Feedback lists feedback-kind events across all tickets, newest first.
// Backed by a direct ledger scan on kind. Measure before adding a
// projection/index (same policy as Timeline).
func (s *Store) Feedback(ctx context.Context, f FeedbackFilter) ([]LedgerEvent, error) {
	if f.Limit <= 0 || f.Limit > 10000 {
		f.Limit = 1000
	}
	rows, err := s.Pool.Query(ctx, `
SELECT `+ledgerCols+`
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
		if err := rows.Scan(&e.ID, &e.ULID, &e.Ticket, &e.TS, &e.Actor, &e.ActorType, &e.Session, &e.Host, &e.Kind, &e.Payload); err != nil {
			return nil, err
		}
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

// LedgerFilter selects events from the global ledger.
type LedgerFilter struct {
	Ticket    string // ticket ULID exact match (resolved by caller)
	Kind      string // event kind exact match
	ActorType string // human | agent | system
	SinceID   int64  // keyset: id > SinceID (catch-up / newer pages)
	BeforeID  int64  // keyset: id < BeforeID (load-older pages)
	Limit     int    // capped 1..10000, default 200
	// Since/Until bound ts as [Since, Until); zero means unbounded.
	Since time.Time
	Until time.Time
	// Tickets matches any of these ticket ULIDs (resolved by caller);
	// combined with Ticket as AND when both are set.
	Tickets []string
	// PerTicket keeps only each ticket's newest PerTicket events before
	// Limit applies, so one query can serve many tickets' recent history.
	PerTicket int
}

const ledgerCols = `id, ulid, ticket_ulid, ts, actor, actor_type::text, session, host, kind, payload`

const ledgerWhere = `
WHERE ($1::bigint IS NULL OR id > $1)
  AND ($2::text IS NULL OR ticket_ulid = $2)
  AND ($3::text IS NULL OR kind = $3)
  AND ($4::text IS NULL OR actor_type::text = $4)
  AND ($5::bigint IS NULL OR id < $5)
  AND ($7::timestamptz IS NULL OR ts >= $7)
  AND ($8::timestamptz IS NULL OR ts < $8)
  AND ($9::text[] IS NULL OR ticket_ulid = ANY($9))`

// Events reads the global ledger, newest first, filtered. This is the
// read path for the journal/digest views and cross-ticket analytics;
// per-ticket History remains the dossier path.
func (s *Store) Events(ctx context.Context, f LedgerFilter) ([]LedgerEvent, error) {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	if f.Limit > 10000 {
		f.Limit = 10000
	}
	q := `SELECT ` + ledgerCols + ` FROM ledger` + ledgerWhere + `
ORDER BY id DESC
LIMIT $6`
	args := []any{nullableI64(f.SinceID), nullableString(f.Ticket), nullableString(f.Kind), nullableString(f.ActorType),
		nullableI64(f.BeforeID), f.Limit, nullableTime(f.Since), nullableTime(f.Until), nullableStrings(f.Tickets)}
	if f.PerTicket > 0 {
		q = `SELECT ` + ledgerCols + ` FROM (
  SELECT ` + ledgerCols + `, row_number() OVER (PARTITION BY ticket_ulid ORDER BY id DESC) AS rn
  FROM ledger` + ledgerWhere + `
) w WHERE rn <= $10
ORDER BY id DESC
LIMIT $6`
		args = append(args, f.PerTicket)
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	evs := []LedgerEvent{}
	for rows.Next() {
		var e LedgerEvent
		if err := rows.Scan(&e.ID, &e.ULID, &e.Ticket, &e.TS, &e.Actor, &e.ActorType, &e.Session, &e.Host, &e.Kind, &e.Payload); err != nil {
			return nil, err
		}
		e.Class = EventClass(e.Kind)
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullableStrings(v []string) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func nullableI64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
