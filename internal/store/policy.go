package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// The rules the store enforces on the live write path, and nowhere else:
// Replay re-projects what was accepted and never re-adjudicates it, so
// history written before a rule existed still rebuilds. Each rule fails
// with one sentinel the API maps to a stable code (the code is not in the
// text: the API prefixes it, and the CLI prints code and message).
var (
	ErrTierRequired        = errors.New("the ticket has no tier; set tier=0..3 first")
	ErrContractRequired    = errors.New("a tier 2+ ticket needs contract_approved before a commit is recorded")
	ErrCriteriaIncomplete  = errors.New("done needs every criterion in state pass, or a human waive with a reason")
	ErrHumanWaiverRequired = errors.New("waiving criteria is a human's call")
)

// policy runs inside the append transaction after the ticket row is locked
// and before apply(), so it reads the read model as it stands at the
// instant the event lands.
func policy(ctx context.Context, tx pgx.Tx, e Event) error {
	p := e.Payload
	switch e.Kind {
	case "status.set":
		switch strField(p, "status") {
		case "active":
			_, err := ticketTier(ctx, tx, e.TicketULID)
			return err
		case "done":
			var open int
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM subitems WHERE ticket_ulid=$1 AND kind='criterion'
				   AND (state IS NULL OR state <> 'pass')`, e.TicketULID).Scan(&open); err != nil {
				return err
			}
			if open == 0 {
				return nil
			}
			if strField(p, "waive") == "" {
				return fmt.Errorf("%w (%d not passed)", ErrCriteriaIncomplete, open)
			}
			if e.ActorType != "human" {
				return ErrHumanWaiverRequired
			}
		}
	case "field.set":
		// a recorded commit is the first evidence of building the store
		// sees; removing the field (v absent) is bookkeeping, not building
		if strField(p, "field") != "head" {
			return nil
		}
		if _, has := p["v"]; !has {
			return nil
		}
		tier, err := ticketTier(ctx, tx, e.TicketULID)
		if err != nil {
			return err
		}
		if tier < 2 {
			return nil
		}
		var approved bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM gate_events WHERE ticket_ulid=$1 AND gate='contract_approved')`,
			e.TicketULID).Scan(&approved); err != nil {
			return err
		}
		if !approved {
			return ErrContractRequired
		}
	}
	return nil
}

// ticketTier reads fields.tier. A JSON 0..3, as a number or as the string
// the CLI's field=value sends, counts; absent or anything else is
// ErrTierRequired, so a client that meant to classify and did not is told
// so rather than silently exempted.
func ticketTier(ctx context.Context, tx pgx.Tx, ticket string) (int, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT fields->'tier' FROM tickets WHERE ulid=$1`, ticket).Scan(&raw); err != nil {
		return 0, err
	}
	tier, ok := parseTier(raw)
	if !ok {
		return 0, ErrTierRequired
	}
	return tier, nil
}

// parseTier accepts the JSON texts 0, 1, 2, 3 and "0", "1", "2", "3", and
// nothing else: no 4, no 1.5, no "two".
func parseTier(raw []byte) (int, bool) {
	switch string(raw) {
	case "0", `"0"`:
		return 0, true
	case "1", `"1"`:
		return 1, true
	case "2", `"2"`:
		return 2, true
	case "3", `"3"`:
		return 3, true
	}
	return 0, false
}
