package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReplayOptions shape one Replay run. DryRun does the whole rebuild inside
// the transaction and rolls it back, so the report is exactly what a real
// run would print. Lossy skips rows that fail to re-apply instead of
// aborting; strict (the default) refuses, because every ledger row applied
// successfully once and a failure now means the projection code changed.
type ReplayOptions struct {
	DryRun bool
	Lossy  bool
}

// ReplayRow names one ledger row the run could not re-apply (lossy only).
type ReplayRow struct {
	ID     int64
	Kind   string
	Reason string
}

// ReplayReport is what a run did, or would do under DryRun.
type ReplayReport struct {
	DryRun    bool
	Rows      int64       // ledger rows re-applied
	Recovered int64       // sub-item identities taken from the snapshot
	Fresh     int64       // sub-item identities minted new (no snapshot match)
	Skipped   []ReplayRow // rows that failed to re-apply (lossy only)
	Watermark int64       // ledger_watermark.last_id after the run
}

// snapKey is the recovery key: unique across every sub-item the read model
// holds (checked on prod 2026-09-29, ticket projection-replay). Body is not
// part of it because subitem.set edits bodies after creation.
type snapKey struct {
	ticket, kind string
	ts           time.Time
}

var errDryRun = errors.New("dry run: rolled back")

// ErrUnrecoverable is returned (wrapped, naming the event) when a strict
// replay over a live read model meets a sub-item that model does not hold.
var ErrUnrecoverable = errors.New("sub-item identity unrecoverable from the read model")

// Replay rebuilds the read model (tickets, slug_history, subitems,
// gate_events, ledger_watermark) from the ledger alone, in one transaction
// that holds ACCESS EXCLUSIVE on the four projection tables so live appends
// wait for it instead of interleaving. The ledger itself is never touched.
//
// Sub-items keep their ULIDs: before truncating, the run snapshots
// (ticket, kind, created_at) -> ulid from the current table and hands those
// back to apply() in place of fresh ones, so subitem.set and subitem.rank
// events that address by prefix still resolve. On an empty read model (a
// restore from a ledger dump) nothing can be recovered; identities are
// minted fresh and reported, and any later prefix reference then fails the
// row — strict refuses, lossy skips and counts.
func (s *Store) Replay(ctx context.Context, opts ReplayOptions) (ReplayReport, error) {
	rep := ReplayReport{DryRun: opts.DryRun}
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL servitor.write = 'on'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`LOCK TABLE tickets, slug_history, subitems, gate_events IN ACCESS EXCLUSIVE MODE`); err != nil {
			return err
		}
		if s.afterReplayLock != nil {
			s.afterReplayLock()
		}

		snap, err := snapshotSubitems(ctx, tx)
		if err != nil {
			return err
		}
		mint := func(ticket, kind string, ts time.Time) (string, error) {
			if u, ok := snap[snapKey{ticket, kind, ts}]; ok {
				rep.Recovered++
				return u, nil
			}
			if len(snap) > 0 && !opts.Lossy {
				return "", fmt.Errorf("%w: %s %s at %s", ErrUnrecoverable, kind, ticket, ts.Format(time.RFC3339Nano))
			}
			rep.Fresh++
			return NewULID(), nil
		}

		if _, err := tx.Exec(ctx, `TRUNCATE tickets, slug_history, subitems, gate_events`); err != nil {
			return err
		}

		// page by id: pgx cannot run apply()'s statements while a result
		// set is open on the same connection, so each page is read whole.
		// Ceiling: the ledger is read in 1000-row pages, fine to millions of
		// rows; trigger to revisit is a page read dominating the run.
		var last int64
		for {
			page, err := readLedgerPage(ctx, tx, last, 1000)
			if err != nil {
				return err
			}
			if len(page) == 0 {
				break
			}
			for _, r := range page {
				sp, err := tx.Begin(ctx) // savepoint: one row's failure is contained
				if err != nil {
					return err
				}
				if err := applyRow(ctx, sp, r.event, r.ts, r.id, mint); err != nil {
					_ = sp.Rollback(ctx)
					if !opts.Lossy {
						return fmt.Errorf("event %d (%s): %w", r.id, r.event.Kind, err)
					}
					rep.Skipped = append(rep.Skipped, ReplayRow{ID: r.id, Kind: r.event.Kind, Reason: err.Error()})
				} else if err := sp.Commit(ctx); err != nil {
					return err
				}
				rep.Rows++
				last = r.id
			}
		}

		if err := tx.QueryRow(ctx,
			`UPDATE ledger_watermark SET last_id=(SELECT coalesce(max(id),0) FROM ledger), updated_at=now()
			 WHERE id=1 RETURNING last_id`).Scan(&rep.Watermark); err != nil {
			return err
		}
		if opts.DryRun {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		return rep, nil
	}
	if err != nil {
		return ReplayReport{}, err
	}
	return rep, nil
}

func snapshotSubitems(ctx context.Context, tx pgx.Tx) (map[snapKey]string, error) {
	rows, err := tx.Query(ctx, `SELECT ticket_ulid, kind, created_at, ulid FROM subitems`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snap := map[snapKey]string{}
	for rows.Next() {
		var k snapKey
		var u string
		if err := rows.Scan(&k.ticket, &k.kind, &k.ts, &u); err != nil {
			return nil, err
		}
		snap[k] = u
	}
	return snap, rows.Err()
}

type ledgerRow struct {
	id    int64
	ts    time.Time
	event Event
}

func readLedgerPage(ctx context.Context, tx pgx.Tx, after int64, limit int) ([]ledgerRow, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, ticket_ulid, ts, actor, actor_type, coalesce(session,''), kind, payload
		 FROM ledger WHERE id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var page []ledgerRow
	for rows.Next() {
		var r ledgerRow
		var payload []byte
		if err := rows.Scan(&r.id, &r.event.TicketULID, &r.ts, &r.event.Actor, &r.event.ActorType,
			&r.event.Session, &r.event.Kind, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &r.event.Payload); err != nil {
			return nil, fmt.Errorf("event %d payload: %w", r.id, err)
		}
		page = append(page, r)
	}
	return page, rows.Err()
}
