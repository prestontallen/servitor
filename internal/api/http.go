// HTTP transport for the Service interface. Deliberately thin: routing,
// status codes, SSE encoding. No business logic lives here.
package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

//go:embed static
var staticFS embed.FS

// HTTP serves the API over net/http. It depends on the Service interface
// only — swap the implementation and this file doesn't change.
type HTTP struct {
	Service Service
	// Token, when set, requires non-local requests to present it as
	// "Authorization: Bearer <token>". Localhost (CLI/hook/GUI on the host)
	// is always allowed. Empty = no auth (dev only).
	Token string
}

func NewHTTP(s Service) *HTTP { return &HTTP{Service: s} }

func (h *HTTP) authed(next http.Handler) http.Handler {
	if h.Token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// static assets are open; only API routes are guarded
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if isLocal(r) {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token") // browser GUI: ?token=
		}
		if token != h.Token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
				"code": "unauthorized", "message": "missing or invalid bearer token"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocal(r *http.Request) bool {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1"
}

// Routes returns the API routes wrapped in auth middleware. Static assets
// are served unauthenticated (the shell is useless without data); API
// routes require the token only for non-local requests when Token is set.
func (h *HTTP) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", h.healthz)
	mux.HandleFunc("GET /api/board", h.board)
	mux.HandleFunc("GET /api/tickets", h.list)
	mux.HandleFunc("GET /api/arcs", h.arcs)
	mux.HandleFunc("GET /api/events", h.events)
	mux.HandleFunc("GET /api/ticket/{ref}", h.ctx)
	mux.HandleFunc("GET /api/ticket/{ref}/history", h.history)
	mux.HandleFunc("POST /api/events", h.append)
	mux.HandleFunc("GET /api/events/stream", h.stream)
	mux.HandleFunc("GET /api/feedback", h.feedback)
	mux.HandleFunc("GET /api/analytics", h.analytics)
	mux.HandleFunc("GET /api/analytics/handoffs", h.handoffs)
	mux.HandleFunc("GET /api/timeline", h.timeline)
	mux.Handle("/", h.static())
	return h.authed(mux)
}

// static serves the embedded GUI (built by web/ via Vite). index.html is
// served no-cache so deploys are picked up immediately; hashed assets
// (/assets/...) are immutable and cached long.
func (h *HTTP) static() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return cacheHeaders(http.StripPrefix("/", http.FileServer(http.FS(sub))))
}

func cacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTP) feedback(w http.ResponseWriter, r *http.Request) {
	f := FeedbackFilter{}
	if s := r.URL.Query().Get("since"); s != "" {
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				f.Since = &t
				break
			}
		}
	}
	f.Source = r.URL.Query().Get("source")
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			f.Limit = n
		}
	}
	evs, err := h.Service.Feedback(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evs)
}

func (h *HTTP) analytics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			days = n
		}
	}
	buckets, err := h.Service.Analytics(r.Context(), days)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buckets)
}

// timeline: /api/timeline?days=N or ?since=&until= (RFC3339 or a bare
// date). Bad timestamps are a 422, never silently ignored.
func (h *HTTP) timeline(w http.ResponseWriter, r *http.Request) {
	q := TimelineQuery{Days: 30}
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			q.Days = n
		}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if s := r.URL.Query().Get("since"); s != "" {
			if t, err := time.Parse(layout, s); err == nil {
				q.Since = &t
				break
			}
		}
	}
	if q.Since == nil {
		if s := r.URL.Query().Get("since"); s != "" {
			writeErr(w, &APIError{Code: "invalid_payload", Message: "since must be RFC3339 or YYYY-MM-DD"})
			return
		}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if s := r.URL.Query().Get("until"); s != "" {
			if t, err := time.Parse(layout, s); err == nil {
				q.Until = &t
				break
			}
		}
	}
	if q.Until == nil {
		if s := r.URL.Query().Get("until"); s != "" {
			writeErr(w, &APIError{Code: "invalid_payload", Message: "until must be RFC3339 or YYYY-MM-DD"})
			return
		}
	}
	tl, err := h.Service.Timeline(r.Context(), q)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tl)
}

// handoffs serves per-ticket human/agent round-trip latency.
func (h *HTTP) handoffs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Service.Handoffs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *APIError
	if !errors.As(err, &ae) {
		ae = &APIError{Code: "internal", Message: err.Error()}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ae.HTTPStatus())
	json.NewEncoder(w).Encode(map[string]any{"error": ae})
}

func (h *HTTP) healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.Ping(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *HTTP) ctx(w http.ResponseWriter, r *http.Request) {
	doc, err := h.Service.Ctx(r.Context(), r.PathValue("ref"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(doc)
}

func (h *HTTP) board(w http.ResponseWriter, r *http.Request) {
	cards, err := h.Service.Board(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cards)
}

func (h *HTTP) arcs(w http.ResponseWriter, r *http.Request) {
	arcs, err := h.Service.Arcs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(arcs)
}

// events serves the global ledger query: ticket (slug or ULID), kind,
// actor_type, since_id (keyset), limit. Newest first.
func (h *HTTP) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.LedgerFilter{
		Ticket:    q.Get("ticket"),
		Kind:      q.Get("kind"),
		ActorType: q.Get("actor_type"),
	}
	if s := q.Get("since_id"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			f.SinceID = n
		} else {
			writeErr(w, &APIError{Code: "invalid_event", Message: "since_id must be an integer"})
			return
		}
	}
	if s := q.Get("before_id"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			f.BeforeID = n
		} else {
			writeErr(w, &APIError{Code: "invalid_event", Message: "before_id must be an integer"})
			return
		}
	}
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			f.Limit = n
		} else {
			writeErr(w, &APIError{Code: "invalid_event", Message: "limit must be an integer"})
			return
		}
	}
	evs, err := h.Service.Events(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evs)
}

// list handles GET /api/tickets — the archive-reaching read. status is
// repeatable and/or comma-separated; q is the slug/title substring;
// limit caps the result.
func (h *HTTP) list(w http.ResponseWriter, r *http.Request) {
	f := store.ListFilter{}
	q := r.URL.Query()
	for _, raw := range q["status"] {
		for _, st := range strings.Split(raw, ",") {
			if st = strings.TrimSpace(strings.ToLower(st)); st != "" {
				f.Statuses = append(f.Statuses, st)
			}
		}
	}
	f.Query = q.Get("q")
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			f.Limit = n
		}
	}
	cards, err := h.Service.List(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cards)
}

func (h *HTTP) history(w http.ResponseWriter, r *http.Request) {
	limit := 10000
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	evs, err := h.Service.History(r.Context(), r.PathValue("ref"), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evs)
}

func (h *HTTP) append(w http.ResponseWriter, r *http.Request) {
	var cmd WriteCmd
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeErr(w, &APIError{Code: "invalid_event", Message: "body must be a JSON event: " + err.Error()})
		return
	}
	// header override: the transport may attest the actor (CLI/MCP pass their
	// identity; browsers can't be trusted with it)
	if a := r.Header.Get("X-Servitor-Actor"); a != "" {
		cmd.Actor = a
	}
	res, err := h.Service.Append(r.Context(), cmd)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res)
}

// stream is SSE: one event per change notification. Consumers treat the
// last event_id as their watermark and resync via /api/ticket/{ref}/history.
func (h *HTTP) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, &APIError{Code: "internal", Message: "streaming unsupported"})
		return
	}
	sub, err := h.Service.Subscribe(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer sub.Cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case c, ok := <-sub.Changes:
			if !ok {
				// notify connection lost: tell the client to resync, then end.
				// the client reconnects and replays past its watermark.
				w.Write([]byte("event: resync\ndata: {}\n\n"))
				fl.Flush()
				return
			}
			b, _ := json.Marshal(c)
			w.Write([]byte("event: change\ndata: " + string(b) + "\n\n"))
			fl.Flush()
		}
	}
}

// RefFromPath is a small helper for transports that need slug/ULID
// normalization ("the/thing" style refs are not supported; refs are either
// a ULID or a bare slug).
func RefFromPath(p string) string {
	return strings.Trim(p, "/")
}

var _ = context.Background
