package graphql

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// maxEvents is the ledger read cap, the same 10000 the store and REST use.
const maxEvents = 10000

// batchWait is how long the first Ticket.events call in a request waits for
// its siblings before reading. gqlgen resolves list elements concurrently,
// so siblings arrive within microseconds; the wait only has to cover that.
var batchWait = 2 * time.Millisecond

// eventsKey is one distinct argument set of Ticket.events. Calls with the
// same key share one ledger read; different keys read separately.
type eventsKey struct {
	since, until time.Time
	kind         string
	limit        int
}

type eventsBatch struct {
	ulids  []string
	seen   map[string]bool
	done   chan struct{}
	byULID map[string][]*store.LedgerEvent
	err    error
}

// eventBatcher turns N Ticket.events calls with one argument set into one
// Service.Events call with Tickets and PerTicket set.
type eventBatcher struct {
	svc     api.Service
	mu      sync.Mutex
	pending map[eventsKey]*eventsBatch
}

func newEventBatcher(svc api.Service) *eventBatcher {
	return &eventBatcher{svc: svc, pending: map[eventsKey]*eventsBatch{}}
}

func (b *eventBatcher) load(ctx context.Context, k eventsKey, ulid string) ([]*store.LedgerEvent, error) {
	b.mu.Lock()
	batch, ok := b.pending[k]
	if !ok {
		batch = &eventsBatch{seen: map[string]bool{}, done: make(chan struct{})}
		b.pending[k] = batch
		go b.flush(context.WithoutCancel(ctx), k, batch)
	}
	if !batch.seen[ulid] {
		batch.seen[ulid] = true
		batch.ulids = append(batch.ulids, ulid)
	}
	b.mu.Unlock()

	select {
	case <-batch.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if batch.err != nil {
		return nil, batch.err
	}
	evs := batch.byULID[ulid]
	if evs == nil {
		evs = []*store.LedgerEvent{}
	}
	return evs, nil
}

func (b *eventBatcher) flush(ctx context.Context, k eventsKey, batch *eventsBatch) {
	time.Sleep(batchWait)
	b.mu.Lock()
	delete(b.pending, k)
	b.mu.Unlock()
	defer close(batch.done)

	if n := len(batch.ulids) * k.limit; n > maxEvents {
		batch.err = &api.APIError{Code: "invalid_event", Message: fmt.Sprintf(
			"nested events would read %d tickets x limit %d = %d, over %d; lower limit or narrow the tickets", len(batch.ulids), k.limit, n, maxEvents)}
		return
	}
	evs, err := b.svc.Events(ctx, store.LedgerFilter{
		Tickets: batch.ulids, Since: k.since, Until: k.until, Kind: k.kind,
		PerTicket: k.limit, Limit: len(batch.ulids) * k.limit,
	})
	if err != nil {
		batch.err = err
		return
	}
	batch.byULID = map[string][]*store.LedgerEvent{}
	for i := range evs {
		e := &evs[i]
		batch.byULID[e.Ticket] = append(batch.byULID[e.Ticket], e)
	}
}
