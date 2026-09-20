package api

import (
	"context"
	"time"
)

// TimelineQuery selects the window. Days is the shorthand (default 30,
// capped at 365); Since/Until override it — both set wins outright, one
// set replaces that edge only. Times parse as RFC3339 or a bare date.
type TimelineQuery struct {
	Days  int
	Since *time.Time
	Until *time.Time
}

// Segment is one continuous stretch of one ticket's life in a single
// phase. Phases are the card-word progression (queued, shaping,
// building, checking, shipping) plus blocked; the derivation mirrors
// the store's own card-word rule (store.go gate mapping) replayed over
// the ledger. Segments are clipped to the requested window; done and
// dropped simply end a ticket's segments.
type Segment struct {
	Ticket string    `json:"ticket_ulid"`
	Slug   string    `json:"slug"`
	Phase  string    `json:"phase"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// HourBucket is one hour of ledger activity split by actor_type — the
// same keys the ledger carries (agent, human, system).
type HourBucket struct {
	Hour    time.Time        `json:"hour"`
	ByActor map[string]int64 `json:"by_actor"`
}

// Timeline is the /api/timeline payload: one read that Flow, Day and
// Cadence can share instead of replaying every ticket's history.
type Timeline struct {
	Since    time.Time    `json:"since"`
	Until    time.Time    `json:"until"`
	Segments []Segment    `json:"segments"`
	Buckets  []HourBucket `json:"buckets"`
}

// gateWord mirrors the store's gate -> card-word mapping exactly.
var gateWord = map[string]string{
	"contract_approved": "building",
	"presented":         "checking",
	"shipped":           "shipping",
}

// Timeline derives the phase segments and hourly actor buckets for the
// window. Direct SQL over the ledger (the analytics pattern): the
// phase-bearing kinds are rare, so segments replay the full history of
// those events — a ticket's phase at the window's start edge is correct
// even when the transition happened long before the window.
func (ss *StoreService) Timeline(ctx context.Context, q TimelineQuery) (Timeline, error) {
	until := time.Now()
	days := q.Days
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	since := until.AddDate(0, 0, -days)
	if q.Since != nil {
		since = *q.Since
	}
	if q.Until != nil {
		until = *q.Until
	}
	if !until.After(since) {
		return Timeline{}, &APIError{Code: "invalid_payload", Message: "until must be after since"}
	}

	out := Timeline{Since: since, Until: until, Segments: []Segment{}, Buckets: []HourBucket{}}

	rows, err := ss.Store.Pool.Query(ctx, `
SELECT l.ticket_ulid, t.slug, l.ts, l.kind,
       COALESCE(l.payload->>'status',''), COALESCE(l.payload->>'gate','')
FROM ledger l JOIN tickets t ON t.ulid = l.ticket_ulid
WHERE l.kind IN ('ticket.create','status.set','gate')
ORDER BY l.ticket_ulid, l.id`)
	if err != nil {
		return Timeline{}, wrapUnreachable(err)
	}
	defer rows.Close()

	type state struct {
		slug  string
		word  string // latest gate's card word, survives blocked detours
		phase string
		start time.Time
		open  bool
	}
	cur := map[string]*state{}
	closeSeg := func(st *state, ulid string, end time.Time) {
		if !st.open {
			return
		}
		st.open = false
		from, to := st.start, end
		if to.Before(since) || from.After(until) {
			return
		}
		if from.Before(since) {
			from = since
		}
		if to.After(until) {
			to = until
		}
		out.Segments = append(out.Segments, Segment{Ticket: ulid, Slug: st.slug, Phase: st.phase, From: from, To: to})
	}

	for rows.Next() {
		var ulid, slug, kind, status, gate string
		var ts time.Time
		if err := rows.Scan(&ulid, &slug, &ts, &kind, &status, &gate); err != nil {
			return Timeline{}, wrapUnreachable(err)
		}
		st := cur[ulid]
		if st == nil {
			st = &state{slug: slug}
			cur[ulid] = st
		}
		next := st.phase
		switch kind {
		case "ticket.create":
			next = "queued"
		case "status.set":
			switch status {
			case "blocked":
				next = "blocked"
			case "active":
				if st.word != "" {
					next = st.word
				} else {
					next = "shaping"
				}
			case "queued":
				next = "queued"
			default: // done, dropped: the segments simply end
				next = ""
			}
		case "gate":
			if w := gateWord[gate]; w != "" {
				st.word = w
				if st.open && st.phase != "" && st.phase != "blocked" {
					next = w // blocked keeps its phase; the word survives it
				}
			}
		}
		if next != st.phase {
			closeSeg(st, ulid, ts)
			st.phase = next
			if next != "" {
				st.start = ts
				st.open = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Timeline{}, wrapUnreachable(err)
	}
	for ulid, st := range cur {
		closeSeg(st, ulid, until)
	}

	brows, err := ss.Store.Pool.Query(ctx, `
SELECT date_trunc('hour', ts), actor_type::text, count(*)
FROM ledger WHERE ts >= $1 AND ts < $2
GROUP BY 1, 2 ORDER BY 1`, since, until)
	if err != nil {
		return Timeline{}, wrapUnreachable(err)
	}
	defer brows.Close()

	byHour := map[time.Time]HourBucket{}
	var order []time.Time
	for brows.Next() {
		var hour time.Time
		var actor string
		var n int64
		if err := brows.Scan(&hour, &actor, &n); err != nil {
			return Timeline{}, wrapUnreachable(err)
		}
		h := hour.Truncate(time.Second)
		b, seen := byHour[h]
		if !seen {
			b = HourBucket{Hour: h, ByActor: map[string]int64{}}
			byHour[h] = b
			order = append(order, h)
		}
		b.ByActor[actor] = n
	}
	if err := brows.Err(); err != nil {
		return Timeline{}, wrapUnreachable(err)
	}
	for _, h := range order {
		out.Buckets = append(out.Buckets, byHour[h])
	}
	return out, nil
}
