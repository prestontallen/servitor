package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/store"
)

// StoreService implements Service against the TimescaleDB store.
// The only implementation today; the interface exists so transports
// never touch the store directly.
type StoreService struct {
	Store *store.Store
}

func NewStoreService(s *store.Store) *StoreService { return &StoreService{Store: s} }

func (ss *StoreService) Ctx(ctx context.Context, ref string) (json.RawMessage, error) {
	doc, err := ss.Store.CtxRead(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isNotFound(err) {
			return nil, &APIError{Code: "unknown_ticket", Message: err.Error()}
		}
		return nil, wrapUnreachable(err)
	}
	return json.RawMessage(doc), nil
}

func (ss *StoreService) Board(ctx context.Context) ([]store.Card, error) {
	cards, err := ss.Store.Board(ctx)
	if err != nil {
		return nil, wrapUnreachable(err)
	}
	if cards == nil {
		cards = []store.Card{} // additive-keys JSON: [] not null for empty sets
	}
	return cards, nil
}

func (ss *StoreService) History(ctx context.Context, ref string, limit int) ([]store.LedgerEvent, error) {
	id, err := ss.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	evs, err := ss.Store.History(ctx, id, limit)
	if err != nil {
		return nil, wrapUnreachable(err)
	}
	if evs == nil {
		evs = []store.LedgerEvent{}
	}
	return evs, nil
}

func (ss *StoreService) Append(ctx context.Context, cmd WriteCmd) (AppendResult, error) {
	if cmd.Kind == "" || cmd.Actor == "" {
		return AppendResult{}, &APIError{Code: "invalid_event",
			Message: "kind and actor are required"}
	}
	actorType := "agent"
	if len(cmd.Actor) > 6 && cmd.Actor[:6] == "human:" {
		actorType = "human"
	} else if cmd.Actor == "system" {
		actorType = "system"
	}
	// ticket.create generates its ULID server-side when not supplied
	if cmd.Ticket == "" {
		if cmd.Kind != "ticket.create" {
			return AppendResult{}, &APIError{Code: "invalid_event",
				Message: "ticket is required for this kind"}
		}
		cmd.Ticket = store.NewULID()
	}
	// feedback is a structured kind: it must carry a finding. Other
	// unknown kinds remain ledger-only, verbatim (store invariant 4).
	if cmd.Kind == "feedback" {
		if err := validateFeedback(cmd.Payload); err != nil {
			return AppendResult{}, err
		}
	}
	ticket, err := ss.resolve(ctx, cmd.Ticket)
	if err != nil {
		return AppendResult{}, err
	}
	expect := time.Time{}
	if cmd.ExpectUpdated != nil {
		expect = *cmd.ExpectUpdated
	}
	eventID, err := ss.Store.AppendEvent(ctx, store.Event{
		TicketULID: ticket,
		Actor:      cmd.Actor,
		ActorType:  actorType,
		Session:    cmd.Session,
		Kind:       cmd.Kind,
		Payload:    cmd.Payload,
	}, expect)
	if err != nil {
		return AppendResult{}, classifyWriteErr(err)
	}
	var updated time.Time
	err = ss.Store.Pool.QueryRow(ctx,
		`SELECT updated_at FROM tickets WHERE ulid=$1`, ticket).Scan(&updated)
	if err != nil {
		return AppendResult{}, wrapUnreachable(err)
	}
	return AppendResult{EventID: eventID, TicketULID: ticket, Updated: updated}, nil
}

// Subscribe uses a dedicated connection on NOTIFY as the fast path.
// The watermark (CtxRead's "head", History keyset) remains the resync
// authority — subscribers replay anything past their last event_id.
func (ss *StoreService) Subscribe(ctx context.Context) (Subscription, error) {
	conn, err := ss.Store.Pool.Acquire(ctx)
	if err != nil {
		return Subscription{}, wrapUnreachable(err)
	}
	if _, err := conn.Exec(ctx, `LISTEN servitor_events`); err != nil {
		conn.Release()
		return Subscription{}, wrapUnreachable(err)
	}
	ch := make(chan Change, 64)
	done := make(chan struct{})
	go func() {
		defer conn.Release()
		defer close(ch)
		for {
			n, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return // context done or connection lost: consumer resyncs via watermark
			}
			var c Change
			if json.Unmarshal([]byte(n.Payload), &c) == nil {
				select {
				case ch <- c:
				case <-done:
					return
				}
			}
		}
	}()
	return Subscription{Changes: ch, Cancel: func() {
		close(done)
		conn.Release()
	}}, nil
}

// Feedback lists feedback-kind ledger events across all tickets.
func (ss *StoreService) Feedback(ctx context.Context, f FeedbackFilter) ([]store.LedgerEvent, error) {
	evs, err := ss.Store.Feedback(ctx, store.FeedbackFilter(f))
	if err != nil {
		return nil, wrapUnreachable(err)
	}
	return evs, nil
}

// validateFeedback enforces the feedback payload shape: finding (required,
// non-empty string), source (optional, human|self, default self).
func validateFeedback(p map[string]any) error {
	finding, _ := p["finding"].(string)
	if finding == "" {
		return &APIError{Code: "invalid_event",
			Message: "feedback requires a non-empty \"finding\" string"}
	}
	switch s, _ := p["source"].(string); s {
	case "", "human", "self":
	default:
		return &APIError{Code: "invalid_event",
			Message: "feedback \"source\" must be human or self"}
	}
	return nil
}

func (ss *StoreService) Ping(ctx context.Context) error {
	if err := ss.Store.Pool.Ping(ctx); err != nil {
		return &APIError{Code: "unreachable", Message: err.Error()}
	}
	return nil
}

// resolve maps a ULID-or-slug reference to a ticket ULID.
func (ss *StoreService) resolve(ctx context.Context, ref string) (string, error) {
	if store.IsULID(ref) {
		return ref, nil
	}
	id, err := ss.Store.TicketBySlug(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &APIError{Code: "no_live_slug", Message: err.Error()}
		}
		return "", wrapUnreachable(err)
	}
	return id, nil
}

func isNotFound(err error) bool {
	return err != nil && (errors.Is(err, pgx.ErrNoRows) ||
		containsAny(err.Error(), "no ticket", "no live ticket"))
}

func classifyWriteErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case errors.Is(err, store.ErrStaleWrite):
		return &APIError{Code: "stale_write", Message: msg}
	case containsAny(msg, `blocked requires "on"`):
		return &APIError{Code: "blocked_requires_on", Message: msg}
	case containsAny(msg, "human actor"):
		return &APIError{Code: "human_gate_required", Message: msg}
	case containsAny(msg, "gate_events_one_per_ticket"):
		return &APIError{Code: "gate_already_passed", Message: msg}
	case containsAny(msg, "claimed by ticket", "claimed (history-wide)"):
		return &APIError{Code: "slug_claimed", Message: msg}
	case containsAny(msg, "ambiguous prefix"):
		return &APIError{Code: "ambiguous_prefix", Message: msg}
	case containsAny(msg, "no subitem matches"):
		return &APIError{Code: "unknown_subitem", Message: msg}
	case containsAny(msg, "invalid status"):
		return &APIError{Code: "invalid_status", Message: msg}
	case containsAny(msg, "requires", "must not be empty", "invalid"):
		return &APIError{Code: "invalid_event", Message: msg}
	default:
		return wrapUnreachable(err)
	}
}

func wrapUnreachable(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &APIError{Code: "unreachable", Message: err.Error()}
	}
	// connection-level failures surface as 5xx with the same envelope
	return &APIError{Code: "internal", Message: err.Error()}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
