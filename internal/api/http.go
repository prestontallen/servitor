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
)

//go:embed static
var staticFS embed.FS

// HTTP serves the API over net/http. It depends on the Service interface
// only — swap the implementation and this file doesn't change.
type HTTP struct {
	Service Service
}

func NewHTTP(s Service) *HTTP { return &HTTP{Service: s} }

func (h *HTTP) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", h.healthz)
	mux.HandleFunc("GET /api/board", h.board)
	mux.HandleFunc("GET /api/ticket/{ref}", h.ctx)
	mux.HandleFunc("GET /api/ticket/{ref}/history", h.history)
	mux.HandleFunc("POST /api/events", h.append)
	mux.HandleFunc("GET /api/events/stream", h.stream)
	mux.HandleFunc("GET /api/analytics", h.analytics)
	mux.Handle("/", h.static())
	return mux
}

// static serves the embedded GUI (single-page, no build step).
func (h *HTTP) static() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/", http.FileServer(http.FS(sub)))
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
