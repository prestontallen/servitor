package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
