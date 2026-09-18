package store

import (
	"context"
	"time"
)

// HandoffRow is one ticket's human/agent round-trip latency, measured on
// the ledger. human_wait = presented gate ts − last agent signal before
// it (how long the work sat waiting for the human). agent_wait = agent's
// first signal after contract_approved − contract_approved ts (how long
// the human waited for the agent to pick the approved contract back up).
// Either side is nil when its endpoint never happened.
type HandoffRow struct {
	ULID          string     `json:"ulid"`
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	PresentedAt   *time.Time `json:"presented_at"`
	ApprovedAt    *time.Time `json:"approved_at"`
	HumanWaitSecs *float64   `json:"human_wait_secs"`
	AgentWaitSecs *float64   `json:"agent_wait_secs"`
}

// Handoffs returns handoff latency for every ticket that reached
// presented, newest presentation first. Direct ledger/gate_events scan —
// same measure-first policy as Analytics/Feedback.
func (s *Store) Handoffs(ctx context.Context) ([]HandoffRow, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT t.ulid, t.slug, t.title,
       g.ts,
       a.ts,
       (SELECT EXTRACT(EPOCH FROM g.ts - max(l.ts))
          FROM ledger l
         WHERE l.ticket_ulid = t.ulid
           AND l.actor_type = 'agent'
           AND l.kind IN ('note','decision','feedback')
           AND l.ts < g.ts),
       (SELECT EXTRACT(EPOCH FROM min(l.ts) - a.ts)
          FROM ledger l
         WHERE l.ticket_ulid = t.ulid
           AND l.actor_type = 'agent'
           AND l.kind IN ('note','decision')
           AND l.ts > a.ts)
FROM tickets t
JOIN gate_events g ON g.ticket_ulid = t.ulid AND g.gate = 'presented'
LEFT JOIN gate_events a ON a.ticket_ulid = t.ulid AND a.gate = 'contract_approved'
ORDER BY g.ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HandoffRow{}
	for rows.Next() {
		var r HandoffRow
		if err := rows.Scan(&r.ULID, &r.Slug, &r.Title,
			&r.PresentedAt, &r.ApprovedAt, &r.HumanWaitSecs, &r.AgentWaitSecs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
