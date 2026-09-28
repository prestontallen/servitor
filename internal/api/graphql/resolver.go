// Package graphql is servitor's read-only GraphQL surface: a gqlgen schema
// (schema.graphqls) resolved over api.Service, the same interface the REST
// routes use. It never touches the store directly and never writes; writes
// stay on POST /api/events. Regenerate with `go tool gqlgen generate`.
package graphql

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// Resolver is the gqlgen root. Per-request state lives in loaders, which
// the handler puts on the request context.
type Resolver struct {
	Service api.Service
}

// loaders is one request's cache: the ticket index (read once, on first
// need) and the batcher for nested Ticket.events.
type loaders struct {
	svc api.Service

	once     sync.Once
	indexErr error
	byULID   map[string]*store.Card
	bySlug   map[string]*store.Card
	children map[string][]*store.Card

	events *eventBatcher
}

type loadersKey struct{}

func withLoaders(ctx context.Context, svc api.Service) context.Context {
	return context.WithValue(ctx, loadersKey{}, &loaders{svc: svc, events: newEventBatcher(svc)})
}

func loadersFrom(ctx context.Context) *loaders {
	return ctx.Value(loadersKey{}).(*loaders)
}

// index reads every ticket once per request. Parent, children, event and
// arc-member links resolve through it, so a nested query does not pay one
// read per link. Ceiling: the whole ticket table per request (95 rows
// today); trigger to revisit: a request profile where this read dominates.
func (l *loaders) index(ctx context.Context) error {
	l.once.Do(func() {
		cards, err := l.svc.List(ctx, store.ListFilter{})
		if err != nil {
			l.indexErr = err
			return
		}
		l.byULID = make(map[string]*store.Card, len(cards))
		l.bySlug = make(map[string]*store.Card, len(cards))
		l.children = map[string][]*store.Card{}
		for i := range cards {
			c := &cards[i]
			l.byULID[c.ULID] = c
			l.bySlug[c.Slug] = c
			if c.Parent != nil {
				l.children[*c.Parent] = append(l.children[*c.Parent], c)
			}
		}
	})
	return l.indexErr
}

// card returns the ticket with this ULID from the index.
func (l *loaders) card(ctx context.Context, ulid string) (*store.Card, error) {
	if err := l.index(ctx); err != nil {
		return nil, err
	}
	c, ok := l.byULID[ulid]
	if !ok {
		return nil, &api.APIError{Code: "unknown_ticket", Message: "no ticket " + ulid}
	}
	return c, nil
}

// resolve turns a ULID or slug into a ticket. Live slugs and ULIDs hit the
// index; anything else (an old slug, an unknown ref) goes through
// Service.Ctx, which knows slug history and returns unknown_ticket.
func (l *loaders) resolve(ctx context.Context, ref string) (*store.Card, error) {
	if err := l.index(ctx); err != nil {
		return nil, err
	}
	if c, ok := l.byULID[ref]; ok {
		return c, nil
	}
	if c, ok := l.bySlug[ref]; ok {
		return c, nil
	}
	doc, err := l.svc.Ctx(ctx, ref)
	if err != nil {
		return nil, err
	}
	var head struct {
		ULID string `json:"ulid"`
	}
	if err := json.Unmarshal(doc, &head); err != nil {
		return nil, err
	}
	return l.card(ctx, head.ULID)
}

func ptrs(cards []store.Card) []*store.Card {
	out := make([]*store.Card, len(cards))
	for i := range cards {
		out[i] = &cards[i]
	}
	return out
}
