package store

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Event is one ledger append. Payload is the full command intent.
type Event struct {
	TicketULID string
	Actor      string // "human:preston" | "agent:<id>" | "system"
	ActorType  string // "human" | "agent" | "system"
	Session    string
	Kind       string
	Payload    map[string]any
}

func (e Event) validate() error {
	if !IsULID(e.TicketULID) {
		return fmt.Errorf("ticket ulid invalid: %q", e.TicketULID)
	}
	if e.Kind == "" {
		return errors.New("event kind must not be empty")
	}
	if e.Actor == "" {
		return errors.New("event actor must not be empty")
	}
	switch e.ActorType {
	case "human", "agent", "system":
	default:
		return fmt.Errorf("actor_type must be human|agent|system, got %q", e.ActorType)
	}
	return nil
}

func NewULID() string {
	return ulid.MustNew(ulid.Now(), cryptoRand{}).String()
}

type cryptoRand struct{}

func (cryptoRand) Read(p []byte) (int, error) { return rand.Read(p) }

func IsULID(s string) bool {
	_, err := ulid.ParseStrict(s)
	return err == nil
}

type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

// migration is one ordered schema change. Files are migrations/NNN_name.sql;
// NNN is the version, applied strictly in order.
type migration struct {
	id   int
	name string
	sql  string
}

func loadMigrations() ([]migration, error) {
	paths, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, p := range paths {
		base := strings.TrimPrefix(p, "migrations/")
		var id int
		var name string
		if _, err := fmt.Sscanf(base, "%03d_%s", &id, &name); err != nil {
			return nil, fmt.Errorf("bad migration filename %q (want NNN_name.sql): %v", base, err)
		}
		name = strings.TrimSuffix(name, ".sql")
		b, err := migrationsFS.ReadFile(p)
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{id: id, name: name, sql: string(b)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].id < ms[j].id })
	for i := 1; i < len(ms); i++ {
		if ms[i].id == ms[i-1].id {
			return nil, fmt.Errorf("duplicate migration version %03d", ms[i].id)
		}
	}
	return ms, nil
}

// migrationsTableName tracks applied migrations. It lives in the store package's
// own schema, deliberately outside the projection-only guard (it must be
// writable by the migration runner without the servitor.write GUC).
const migrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	id         int         PRIMARY KEY,
	name       text        NOT NULL,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// ApplySchema brings the database to the latest schema version. On a fresh
// database it applies every migration in order. On a database that predates
// migrations (ledger exists but nothing is recorded) it stamps the baseline
// 001_init.sql as already applied, then applies anything newer. Safe to run
// repeatedly; each migration runs exactly once, in its own transaction.
func (s *Store) ApplySchema(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	applied := map[int]bool{}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		applied[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	// Baseline stamp: a pre-migration database already has the full original
	// schema (001_init.sql). Record it instead of re-running it.
	if len(applied) == 0 {
		var ledgerExists bool
		if err := s.Pool.QueryRow(ctx,
			`SELECT to_regclass('public.ledger') IS NOT NULL`).Scan(&ledgerExists); err != nil {
			return err
		}
		if ledgerExists {
			for _, m := range ms {
				if m.id == 1 {
					if err := s.stampMigration(ctx, m); err != nil {
						return err
					}
					applied[m.id] = true
				}
			}
		}
	}

	for _, m := range ms {
		if applied[m.id] {
			continue
		}
		if err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return fmt.Errorf("migration %03d_%s failed: %w", m.id, m.name, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (id, name) VALUES ($1, $2)`, m.id, m.name); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) stampMigration(ctx context.Context, m migration) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO schema_migrations (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, m.id, m.name)
	return err
}

func payloadJSON(p map[string]any) ([]byte, error) {
	if p == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(p)
}

// AppendEvent applies one event: ledger insert + projection, in ONE transaction.
// This is the sanctioned write path; nothing else may mutate the read model.
// Concurrency: per-ticket serialization via SELECT ... FOR UPDATE on tickets
// (ticket.create inserts the anchor row instead). Clients wanting optimistic
// concurrency pass expectUpdated (zero time = skip check).
func (s *Store) AppendEvent(ctx context.Context, e Event, expectUpdated time.Time) (int64, error) {
	if err := e.validate(); err != nil {
		return 0, err
	}
	pl, err := payloadJSON(e.Payload)
	if err != nil {
		return 0, err
	}
	var eventID int64
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL servitor.write = 'on'`); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO ledger (ulid, ticket_ulid, ts, actor, actor_type, session, kind, payload)
			 VALUES ($1,$2,now(),$3,$4,$5,$6,$7) RETURNING id, ts`,
			NewULID(), e.TicketULID, e.Actor, e.ActorType, e.Session, e.Kind, pl)
		var ts time.Time
		if err := row.Scan(&eventID, &ts); err != nil {
			return err
		}

		// serialize writers per ticket
		if e.Kind != "ticket.create" {
			tag, err := tx.Exec(ctx, `SELECT 1 FROM tickets WHERE ulid=$1 FOR UPDATE`, e.TicketULID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("unknown ticket %s (create it first)", e.TicketULID)
			}
			if !expectUpdated.IsZero() {
				var updated time.Time
				if err := tx.QueryRow(ctx, `SELECT updated_at FROM tickets WHERE ulid=$1`, e.TicketULID).Scan(&updated); err != nil {
					return err
				}
				if !updated.Equal(expectUpdated) {
					return ErrStaleWrite
				}
			}
		}

		if err := apply(ctx, tx, e, ts, eventID); err != nil {
			return err
		}

		// every event touches the aggregate's updated_at (stale-write token)
		if e.Kind != "ticket.create" {
			if _, err := tx.Exec(ctx, `UPDATE tickets SET updated_at=$1 WHERE ulid=$2`, ts, e.TicketULID); err != nil {
				return err
			}
		}

		// watermark + notify ride the same transaction
		if _, err := tx.Exec(ctx, `UPDATE ledger_watermark SET last_id=$1, updated_at=now() WHERE id=1`, eventID); err != nil {
			return err
		}
		notify, _ := json.Marshal(map[string]any{"id": eventID, "ticket": e.TicketULID, "kind": e.Kind})
		_, err = tx.Exec(ctx, `SELECT pg_notify('servitor_events', $1)`, string(notify))
		return err
	})
	if err != nil {
		return 0, err
	}
	return eventID, nil
}

// EventClass buckets every event kind into signal (a human- or
// agent-chosen record: notes, decisions, gates, feedback) vs transition
// (mechanical state movement: creates, status/field sets, subitem
// bookkeeping, hook chatter). Unknown kinds are transitions — the store
// treats unknown kinds as ledger-only verbatim, which is mechanical by
// definition. Views group and weight by class.
func EventClass(kind string) string {
	switch kind {
	case "note", "decision", "gate", "feedback":
		return "signal"
	default:
		return "transition"
	}
}

var ErrStaleWrite = errors.New("stale write: ticket changed since read")

func apply(ctx context.Context, tx pgx.Tx, e Event, ts time.Time, eventID int64) error {
	p := e.Payload
	switch e.Kind {
	case "ticket.create":
		slug, _ := p["slug"].(string)
		if slug == "" {
			return errors.New("ticket.create requires slug")
		}
		// claim first: one mechanism owns slug uniqueness (history-wide)
		if err := claimSlug(ctx, tx, e.TicketULID, slug); err != nil {
			return err
		}
		title, _ := p["title"].(string)
		rank := numField(p, "rank")
		if _, err := tx.Exec(ctx,
			`INSERT INTO tickets (ulid, slug, title, rank, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$5)`, e.TicketULID, slug, title, rank, ts); err != nil {
			return err
		}

	case "field.set":
		field, _ := p["field"].(string)
		if field == "" {
			return errors.New("field.set requires field")
		}
		v, hasV := p["v"]
		switch field {
		case "slug":
			s, _ := v.(string)
			if !hasV || s == "" {
				return errors.New("slug rename requires v")
			}
			if err := claimSlug(ctx, tx, e.TicketULID, s); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE tickets SET slug=$1 WHERE ulid=$2`, s, e.TicketULID)
			return err
		case "pr", "parent":
			// v present => its value ("" = empty); absent => NULL (absent)
			if field == "parent" && hasV {
				s, _ := v.(string)
				if s == "" {
					return errors.New("parent requires a ticket ulid (or absent to clear)")
				}
				if s == e.TicketULID {
					return errors.New("ticket cannot be its own parent")
				}
				// the parent must exist, and must not be a descendant of
				// this ticket (cycle guard — a cycle would corrupt rollups).
				// Walk UP from the proposed parent: if this ticket is among
				// its ancestors, assigning would make it its own ancestor.
				var n int
				if err := tx.QueryRow(ctx,
					`WITH RECURSIVE up AS (
					   SELECT ulid, parent FROM tickets WHERE ulid=$2
					   UNION ALL
					   SELECT t.ulid, t.parent FROM tickets t JOIN up ON t.ulid=up.parent
					 ) SELECT count(*) FROM up WHERE ulid=$1`,
					e.TicketULID, s).Scan(&n); err != nil {
					return err
				}
				if n > 0 {
					return fmt.Errorf("parent %s would create a cycle", s)
				}
				tag, err := tx.Exec(ctx, `SELECT 1 FROM tickets WHERE ulid=$1`, s)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					return fmt.Errorf("unknown parent ticket %s", s)
				}
			}
			if !hasV {
				q := `UPDATE tickets SET pr=NULL WHERE ulid=$1`
				if field == "parent" {
					q = `UPDATE tickets SET parent=NULL WHERE ulid=$1`
				}
				_, err := tx.Exec(ctx, q, e.TicketULID)
				return err
			}
			s, _ := v.(string)
			col := "pr"
			if field == "parent" {
				col = "parent"
			}
			_, err := tx.Exec(ctx, `UPDATE tickets SET `+col+`=$1 WHERE ulid=$2`, s, e.TicketULID)
			return err
		default:
			if !hasV { // absent: remove key
				_, err := tx.Exec(ctx, `UPDATE tickets SET fields = fields - $1 WHERE ulid=$2`, field, e.TicketULID)
				return err
			}
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE tickets SET fields = jsonb_set(fields, ARRAY[$1], $2) WHERE ulid=$3`, field, raw, e.TicketULID)
			return err
		}

	case "status.set":
		st, _ := p["status"].(string)
		switch st {
		case "queued", "active", "done", "dropped":
		case "blocked":
			on, _ := p["on"].(string)
			if on == "" {
				return errors.New(`blocked requires "on" (human|named party)`)
			}
		default:
			return fmt.Errorf("invalid status %q", st)
		}
		if st == "blocked" {
			on, _ := p["on"].(string)
			_, err := tx.Exec(ctx,
				`UPDATE tickets SET status='blocked', blocked_on=$1, blocked_since=$2 WHERE ulid=$3`,
				on, ts, e.TicketULID)
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE tickets SET status=$1, blocked_on=NULL, blocked_since=NULL WHERE ulid=$2`,
			st, e.TicketULID)
		return err

	case "gate":
		g, _ := p["gate"].(string)
		switch g {
		case "contract_approved", "presented", "shipped":
		default:
			return fmt.Errorf("invalid gate %q", g)
		}
		if g == "contract_approved" && e.ActorType != "human" {
			return errors.New("contract_approved requires human actor")
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO gate_events (ticket_ulid, gate, actor, actor_type, ts, ledger_id)
			 VALUES ($1,$2,$3,$4,$5,$6)`, e.TicketULID, g, e.Actor, e.ActorType, ts, eventID); err != nil {
			return err
		}
		card := map[string]string{"contract_approved": "building", "presented": "checking", "shipped": "shipping"}[g]
		_, err := tx.Exec(ctx,
			`UPDATE tickets SET card_word=$1, last_gate_ts=$2 WHERE ulid=$3`,
			card, ts, e.TicketULID)
		return err

	case "subitem.add":
		kind, _ := p["kind"].(string)
		body, _ := p["body"].(string)
		state, _ := p["state"].(string)
		if kind == "" {
			return errors.New("subitem.add requires kind")
		}
		sub := NewULID()
		if s, ok := p["ulid"].(string); ok && s != "" {
			sub = s
		}
		fields := p["fields"]
		if fields == nil {
			fields = map[string]any{}
		}
		fj, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO subitems (ulid, ticket_ulid, kind, rank, body, state, fields, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$8)`,
			sub, e.TicketULID, kind, numField(p, "rank"), body, state, fj, ts)
		return err

	case "subitem.set":
		sub, _ := p["ulid"].(string)
		if sub == "" {
			return errors.New("subitem.set requires ulid")
		}
		_, err := tx.Exec(ctx,
			`UPDATE subitems SET
			   body = CASE WHEN $3::jsonb ? 'body'  THEN $4 ELSE body END,
			   state = CASE WHEN $3::jsonb ? 'state' THEN $5 ELSE state END,
			   updated_at = $6
			 WHERE ulid = $2 AND ticket_ulid = $1`,
			e.TicketULID, sub, mustJSON(p), strField(p, "body"), strField(p, "state"), ts)
		return err

	case "subitem.rank":
		prefix, _ := p["ulid"].(string)
		sub, err := resolveSubitem(ctx, tx, e.TicketULID, prefix)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE subitems SET rank=$1, updated_at=$2 WHERE ulid=$3`,
			numField(p, "rank"), ts, sub)
		return err

	case "decision":
		what, _ := p["what"].(string)
		why, _ := p["why"].(string)
		if what == "" || why == "" {
			return errors.New("decision requires what and why")
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO subitems (ulid, ticket_ulid, kind, rank, body, fields, created_at, updated_at)
			 VALUES ($1,$2,'decision',0,$3::text,jsonb_build_object('why',$4::text),$5,$5)
			 ON CONFLICT DO NOTHING`,
			NewULID(), e.TicketULID, what, why, ts)
		return err

	case "note":
		v, _ := p["v"].(string)
		_, err := tx.Exec(ctx,
			`INSERT INTO subitems (ulid, ticket_ulid, kind, rank, body, created_at, updated_at)
			 VALUES ($1,$2,'note',$3,$4,$5,$5)`,
			NewULID(), e.TicketULID, ts.UnixMilli(), v, ts)
		return err

	default:
		// unknown kinds are ledger-only, verbatim (invariant 4)
		return nil
	}
	_ = ts
	return nil
}

// gate ledger_id backfill is handled by Gate using RETURNING — see AppendEventGate.
// (kept simple here: gate_events.ledger_id is informational)

func claimSlug(ctx context.Context, tx pgx.Tx, ticket, slug string) error {
	lower := lowerString(slug)
	var owner string
	err := tx.QueryRow(ctx, `SELECT ticket_ulid FROM slug_history WHERE slug_lower=$1 FOR UPDATE`, lower).Scan(&owner)
	if err == nil {
		if owner != ticket {
			return fmt.Errorf("slug %q claimed by ticket %s (history-wide)", slug, owner)
		}
		_, err = tx.Exec(ctx, `UPDATE slug_history SET released_at=NULL WHERE slug_lower=$1`, lower)
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO slug_history (slug_lower, ticket_ulid) VALUES ($1,$2)`, lower, ticket)
		return err
	}
	return err
}

func numField(p map[string]any, k string) int64 {
	switch v := p[k].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func strField(p map[string]any, k string) string {
	s, _ := p[k].(string)
	return s
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func lowerString(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
