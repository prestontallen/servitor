package graphql

import (
	"context"
	"errors"
	"net/http"

	gql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Query caps, checked before any resolver runs. Complexity is gqlgen's
// default count (one per selected field); depth counts nested selections,
// so a parent_ticket/children cycle cannot recurse without bound.
const (
	MaxComplexity = 500
	MaxDepth      = 8
	maxBodyBytes  = 1 << 20
)

// NewHandler serves POST /api/graphql over svc. The api HTTP skin mounts it
// under its /api/ auth guard; this package adds no auth of its own.
func NewHandler(svc api.Service) http.Handler {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Service: svc}}))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.FixedComplexityLimit(MaxComplexity))
	srv.Use(depthLimit{max: MaxDepth})
	srv.SetErrorPresenter(presentError)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		srv.ServeHTTP(w, r.WithContext(withLoaders(r.Context(), svc)))
	})
}

// presentError carries an APIError's stable code into extensions.code, the
// same codes REST returns in {"error":{"code"}}.
func presentError(ctx context.Context, err error) *gqlerror.Error {
	gerr := gql.DefaultErrorPresenter(ctx, err)
	var ae *api.APIError
	if errors.As(err, &ae) {
		gerr.Message = ae.Message
		if gerr.Extensions == nil {
			gerr.Extensions = map[string]any{}
		}
		gerr.Extensions["code"] = ae.Code
	}
	return gerr
}

// ticketScope turns the events tickets and arc arguments into ticket ULIDs.
// nil means no ticket filter; an empty, non-nil slice means the filters
// share no ticket. The arc scope is the arc ticket plus its non-dropped
// members.
func (r *queryResolver) ticketScope(ctx context.Context, refs []string, arc *string) ([]string, error) {
	l := loadersFrom(ctx)
	var fromTickets, fromArc []string
	for _, ref := range refs {
		c, err := l.resolve(ctx, ref)
		if err != nil {
			return nil, err
		}
		fromTickets = append(fromTickets, c.ULID)
	}
	if arc != nil {
		c, err := l.resolve(ctx, *arc)
		if err != nil {
			return nil, err
		}
		fromArc = []string{c.ULID}
		arcs, err := r.Service.Arcs(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range arcs {
			if a.ULID == c.ULID {
				for _, m := range a.Members {
					fromArc = append(fromArc, m.ULID)
				}
			}
		}
	}
	switch {
	case refs == nil && arc == nil:
		return nil, nil
	case arc == nil:
		return fromTickets, nil
	case refs == nil:
		return fromArc, nil
	}
	in := map[string]bool{}
	for _, u := range fromArc {
		in[u] = true
	}
	both := []string{}
	for _, u := range fromTickets {
		if in[u] {
			both = append(both, u)
		}
	}
	return both, nil
}

// depthLimit rejects operations whose selections nest deeper than max.
type depthLimit struct{ max int }

var _ interface {
	gql.HandlerExtension
	gql.OperationContextMutator
} = depthLimit{}

func (depthLimit) ExtensionName() string { return "DepthLimit" }

func (depthLimit) Validate(gql.ExecutableSchema) error { return nil }

func (d depthLimit) MutateOperationContext(ctx context.Context, oc *gql.OperationContext) *gqlerror.Error {
	if oc.Operation == nil {
		return nil
	}
	if depth := selectionDepth(oc.Operation.SelectionSet, oc.Doc.Fragments, 0); depth > d.max {
		err := gqlerror.Errorf("query depth %d exceeds the limit of %d", depth, d.max)
		err.Extensions = map[string]any{"code": "query_too_deep"}
		return err
	}
	return nil
}

func selectionDepth(set ast.SelectionSet, frags ast.FragmentDefinitionList, seen int) int {
	if seen > 64 {
		return seen // a fragment cycle; validation rejects it anyway
	}
	deepest := seen
	for _, sel := range set {
		var d int
		switch s := sel.(type) {
		case *ast.Field:
			if len(s.SelectionSet) == 0 {
				d = seen + 1
			} else {
				d = selectionDepth(s.SelectionSet, frags, seen+1)
			}
		case *ast.InlineFragment:
			d = selectionDepth(s.SelectionSet, frags, seen)
		case *ast.FragmentSpread:
			if f := frags.ForName(s.Name); f != nil {
				d = selectionDepth(f.SelectionSet, frags, seen)
			}
		}
		if d > deepest {
			deepest = d
		}
	}
	return deepest
}
