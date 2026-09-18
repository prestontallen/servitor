package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// resolveSubitem is the package-level resolver used inside the write path.
func resolveSubitem(ctx context.Context, tx pgx.Tx, ticketULID, prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("empty subitem prefix")
	}
	rows, err := tx.Query(ctx,
		`SELECT ulid FROM subitems WHERE ticket_ulid=$1 AND ulid LIKE $2`,
		ticketULID, strings.ReplaceAll(prefix, "%", "")+"%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", fmt.Errorf("no subitem matches prefix %q", prefix)
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d subitems", prefix, len(ids))
	}
}

// ResolveSubitem maps a ULID prefix to the single matching sub-item of a
// ticket. Ambiguous or unknown prefixes are errors — never position, always
// identity (invariant 2).
func (s *Store) ResolveSubitem(ctx context.Context, tx pgx.Tx, ticketULID, prefix string) (string, error) {
	return resolveSubitem(ctx, tx, ticketULID, prefix)
}

// TicketBySlug resolves a slug (case-insensitive) to its ticket ULID.
func (s *Store) TicketBySlug(ctx context.Context, slug string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`SELECT ticket_ulid FROM slug_history WHERE slug_lower=lower($1) AND released_at IS NULL`, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("no live ticket owns slug %q", slug)
	}
	return id, err
}

// Card is one board row (W2). Parent is set when the card belongs to an
// arc (additive key). UpdatedAt powers aging display. Arcs themselves are
// excluded — see Arcs.
type Card struct {
	ULID      string     `json:"ulid"`
	Slug      string     `json:"slug"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Rank      int64      `json:"rank"`
	CardWord  *string    `json:"card_word"`
	BlockedOn *string    `json:"blocked_on"`
	BlockedAt *time.Time `json:"blocked_since"`
	UpdatedAt *time.Time `json:"updated_at"`
	Parent    *string    `json:"parent"`
}

// Board returns queued/active/blocked cards in rank order. Arcs (tickets
// that are the parent of at least one other ticket) are excluded here —
// they are read through Arcs so the board stays a list of workable cards.
func (s *Store) Board(ctx context.Context) ([]Card, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT ulid, slug, title, status::text, rank, card_word::text, blocked_on, blocked_since, updated_at, parent
FROM tickets
WHERE status IN ('queued','active','blocked')
  AND ulid NOT IN (SELECT parent FROM tickets WHERE parent IS NOT NULL)
ORDER BY CASE status WHEN 'blocked' THEN 0 WHEN 'active' THEN 1 ELSE 2 END, rank`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ULID, &c.Slug, &c.Title, &c.Status, &c.Rank, &c.CardWord, &c.BlockedOn, &c.BlockedAt, &c.UpdatedAt, &c.Parent); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// ListFilter selects tickets for List. Statuses empty means no status
// filter (all five); Query matches case-insensitively against slug and
// title; Limit <= 0 means unlimited.
type ListFilter struct {
	Statuses []string
	Query    string
	Limit    int
}

// validStatuses is the exact set accepted by status.set and List.
var validStatuses = map[string]bool{
	"queued": true, "active": true, "blocked": true, "done": true, "dropped": true,
}

// List returns tickets matching the filter, rank-ordered. Unlike Board it
// reaches done and dropped tickets — including dropped arc members — and
// includes arcs themselves. Validation is strict: an unknown status is an
// error, never a silent empty result.
func (s *Store) List(ctx context.Context, f ListFilter) ([]Card, error) {
	for _, st := range f.Statuses {
		if !validStatuses[st] {
			return nil, fmt.Errorf("invalid status %q", st)
		}
	}
	q := `SELECT ulid, slug, title, status::text, rank, card_word::text, blocked_on, blocked_since, updated_at, parent FROM tickets`
	var conds []string
	var args []any
	if len(f.Statuses) > 0 {
		args = append(args, f.Statuses)
		conds = append(conds, fmt.Sprintf("status::text = ANY($%d)", len(args)))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		conds = append(conds, fmt.Sprintf("(slug ILIKE $%d OR title ILIKE $%d)", len(args), len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY rank"
	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ULID, &c.Slug, &c.Title, &c.Status, &c.Rank, &c.CardWord, &c.BlockedOn, &c.BlockedAt, &c.UpdatedAt, &c.Parent); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// History returns the full event timeline for one ticket (W3).
type LedgerEvent struct {
	ID        int64          `json:"id"`
	ULID      string         `json:"ulid"`
	Ticket    string         `json:"ticket_ulid"`
	TS        time.Time      `json:"ts"`
	Actor     string         `json:"actor"`
	ActorType string         `json:"actor_type"`
	Session   *string        `json:"session"`
	Kind      string         `json:"kind"`
	Class     string         `json:"class"` // signal | transition (derived from kind)
	Payload   map[string]any `json:"payload"`
}

func (s *Store) History(ctx context.Context, ticketULID string, limit int) ([]LedgerEvent, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	rows, err := s.Pool.Query(ctx, `
SELECT id, ulid, ticket_ulid, ts, actor, actor_type::text, session, kind, payload
FROM ledger WHERE ticket_ulid=$1 ORDER BY id LIMIT $2`, ticketULID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var evs []LedgerEvent
	for rows.Next() {
		var e LedgerEvent
		if err := rows.Scan(&e.ID, &e.ULID, &e.Ticket, &e.TS, &e.Actor, &e.ActorType, &e.Session, &e.Kind, &e.Payload); err != nil {
			return nil, err
		}
		e.Class = EventClass(e.Kind)
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

// ArcMember is one ticket under an arc (identity + enough state to render).
type ArcMember struct {
	ULID   string `json:"ulid"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// ArcSummary is one arc (a ticket with member tickets) with its derived
// rollup. Rollup and last_activity consider only non-dropped members; an
// arc's own status is never set by hand.
type ArcSummary struct {
	ULID         string      `json:"ulid"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	Status       string      `json:"status"`        // the arc ticket's own status
	CardWord     *string     `json:"card_word"`     // the arc's own gate word
	Rollup       string      `json:"rollup"`        // queued|active|blocked|done from members
	LastActivity *time.Time  `json:"last_activity"` // newest member update
	Members      []ArcMember `json:"members"`       // non-dropped members
}

// Arcs lists every ticket that is the parent of at least one other ticket,
// with its derived rollup. An arc with no children is indistinguishable
// from a plain ticket (and shows on the board as one) until something
// points at it.
func (s *Store) Arcs(ctx context.Context) ([]ArcSummary, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT t.ulid, t.slug, t.title, t.status::text, t.card_word::text,
  COALESCE(r.rollup, 'queued'), r.last_activity, r.members
FROM tickets t
JOIN (SELECT DISTINCT parent FROM tickets WHERE parent IS NOT NULL) p ON p.parent = t.ulid
LEFT JOIN LATERAL (
  SELECT
    max(m.updated_at) AS last_activity,
    CASE
      WHEN bool_or(m.status = 'blocked') THEN 'blocked'
      WHEN bool_or(m.status = 'active')  THEN 'active'
      WHEN bool_or(m.status <> 'done')   THEN 'queued'
      ELSE 'done'
    END AS rollup,
    jsonb_agg(jsonb_build_object(
      'ulid', m.ulid, 'slug', m.slug, 'title', m.title, 'status', m.status)) AS members
  FROM tickets m
  WHERE m.parent = t.ulid AND m.status <> 'dropped'
) r ON true
ORDER BY r.last_activity DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var arcs []ArcSummary
	for rows.Next() {
		var a ArcSummary
		var members []ArcMember
		if err := rows.Scan(&a.ULID, &a.Slug, &a.Title, &a.Status, &a.CardWord, &a.Rollup, &a.LastActivity, &members); err != nil {
			return nil, err
		}
		if members == nil {
			members = []ArcMember{}
		}
		a.Members = members
		arcs = append(arcs, a)
	}
	return arcs, rows.Err()
}

// CtxRead returns the whole ticket aggregate as one JSON document — the
// SessionStart-hook payload and the API's ctx read (W1).
func (s *Store) CtxRead(ctx context.Context, ticketOrSlug string) ([]byte, error) {
	ticket := ticketOrSlug
	if !IsULID(ticketOrSlug) {
		var err error
		ticket, err = s.TicketBySlug(ctx, ticketOrSlug)
		if err != nil {
			return nil, err
		}
	}
	var doc []byte
	err := s.Pool.QueryRow(ctx, `
WITH t AS (SELECT * FROM tickets WHERE ulid=$1)
SELECT jsonb_build_object(
  'ulid', t.ulid, 'slug', t.slug, 'title', t.title, 'status', t.status,
  'rank', t.rank, 'blocked_on', t.blocked_on, 'blocked_since', t.blocked_since,
  'pr', t.pr, 'fields', t.fields, 'card_word', t.card_word, 'updated_at', t.updated_at,
  'parent', t.parent,
  'gates', COALESCE((SELECT jsonb_agg(jsonb_build_object('gate', g.gate, 'actor', g.actor, 'ts', g.ts) ORDER BY g.ts)
                      FROM gate_events g WHERE g.ticket_ulid=t.ulid), '[]'::jsonb),
  'criteria', COALESCE((SELECT jsonb_agg(jsonb_build_object('ulid',s.ulid,'body',s.body,'state',s.state) ORDER BY s.rank)
                         FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='criterion'), '[]'::jsonb),
  'plan', COALESCE((SELECT jsonb_agg(jsonb_build_object('ulid',s.ulid,'body',s.body) ORDER BY s.rank)
                     FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='plan'), '[]'::jsonb),
  'decisions', COALESCE((SELECT jsonb_agg(jsonb_build_object('ulid',s.ulid,'what',s.body,'why',s.fields->>'why'))
                          FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='decision'), '[]'::jsonb),
  'notes', COALESCE((SELECT jsonb_agg(jsonb_build_object('body',s.body,'ts',s.created_at) ORDER BY s.created_at DESC)
                      FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='note'), '[]'::jsonb),
  'links', COALESCE((SELECT jsonb_agg(jsonb_build_object('ulid',s.ulid,'url',s.state))
                      FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='link'), '[]'::jsonb),
  'questions', COALESCE((SELECT jsonb_agg(jsonb_build_object('ulid',s.ulid,'body',s.body))
                          FROM subitems s WHERE s.ticket_ulid=t.ulid AND s.kind='question'), '[]'::jsonb),
  'feedback', COALESCE((SELECT jsonb_agg(jsonb_build_object('id',l.id,'finding',l.payload->>'finding',
                             'source',COALESCE(l.payload->>'source','self'),'actor',l.actor,'ts',l.ts) ORDER BY l.id DESC)
                        FROM ledger l WHERE l.ticket_ulid=t.ulid AND l.kind='feedback'), '[]'::jsonb),
  'head', (SELECT max(id) FROM ledger WHERE ticket_ulid=t.ulid)
) FROM t`, ticket).Scan(&doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no ticket %q", ticketOrSlug)
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}
