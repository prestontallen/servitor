// Package api defines Servitor's API surface as an interface so the
// transport (HTTP today, MCP or gRPC later) is a thin, swappable skin.
//
// Conventions (ratified):
//   - Responses are raw resource JSON; no envelope. Errors are
//     {"error":{"code","message"}} with stable string codes.
//   - Additive-keys contract: field names may be added, never removed or
//     repurposed. No version field; single-user system.
//   - Field names snake_case; enum values lowercase.
//   - Writes are ledger events: {ticket, kind, payload, actor, session,
//     expect_updated}. The event IS the write unit.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

// APIError is the stable error shape. Code values are contract; messages are human.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// HTTPStatus maps an APIError onto the transport's status codes. The
// interface is transport-agnostic; this helper is for the HTTP skin.
func (e *APIError) HTTPStatus() int {
	switch e.Code {
	case "unknown_ticket", "unknown_subitem", "no_live_slug":
		return 404
	case "stale_write", "slug_claimed":
		return 409
	case "invalid_event", "blocked_requires_on", "human_gate_required",
		"gate_already_passed", "ambiguous_prefix", "invalid_status", "invalid_payload":
		return 422
	case "unreachable":
		return 503
	default:
		return 500
	}
}

func apiErr(code string, err error) *APIError {
	if ae, ok := err.(*APIError); ok {
		return ae
	}
	return &APIError{Code: code, Message: err.Error()}
}

// WriteCmd is one event append request. ExpectUpdated enables optimistic
// concurrency; zero means skip the check.
type WriteCmd struct {
	Ticket        string         `json:"ticket"` // ULID or slug
	Kind          string         `json:"kind"`
	Payload       map[string]any `json:"payload"`
	Actor         string         `json:"actor"`       // "human:preston" | "agent:<id>" | "system"
	Session       string         `json:"session"`
	ExpectUpdated *time.Time     `json:"expect_updated"`
}

// AppendResult reports the committed event.
type AppendResult struct {
	EventID   int64  `json:"event_id"`
	TicketULID string `json:"ticket_ulid"`
	Updated   time.Time `json:"updated"`
}

// Change is one change-detection notification (SSE payload).
type Change struct {
	EventID int64  `json:"event_id"`
	Ticket  string `json:"ticket"`
	Kind    string `json:"kind"`
}

// Subscription is a live change stream. Cancel stops it.
type Subscription struct {
	Changes <-chan Change
	Cancel  func()
}

// Service is THE API surface. Transports depend on this interface only;
// the store-backed implementation is swappable.
type Service interface {
	// Ctx returns the whole ticket aggregate (W1) by ULID or slug.
	Ctx(ctx context.Context, ref string) (json.RawMessage, error)
	// Board returns queued/active/blocked cards in rank order (W2).
	Board(ctx context.Context) ([]store.Card, error)
	// History returns one ticket's event timeline (W3).
	History(ctx context.Context, ref string, limit int) ([]store.LedgerEvent, error)
	// Append applies one ledger event (the write path).
	Append(ctx context.Context, cmd WriteCmd) (AppendResult, error)
	// Subscribe streams change notifications (NOTIFY fast path).
	Subscribe(ctx context.Context) (Subscription, error)
	// Ping reports reachability (hook degrade path: never blocks a session).
	Ping(ctx context.Context) error
}

// compile-time assertion that the store-backed service exists
var _ Service = (*StoreService)(nil)

var ErrUnreachable = errors.New("store unreachable")
