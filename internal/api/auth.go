package api

// Credential authentication for the HTTP transport: user login (session
// cookie) and per-actor API tokens, layered under the existing bearer
// middleware. Actor binding is the point: a credential from this table
// fixes the actor of every write it authorizes — a remote caller cannot
// present X-Servitor-Actor and claim a different identity. The localhost
// CLI keeps today's behaviour (exempt, header attested) and SERVITOR_TOKEN
// keeps its bootstrap role with legacy semantics.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/prestontallen/servitor/internal/store"
)

const sessionCookie = "servitor_session"

// Authenticator holds the credential store. Nil HTTP.Auth means legacy
// behaviour only (SERVITOR_TOKEN / localhost).
type Authenticator struct {
	Store *store.Store
}

type boundActorKeyType struct{}

var boundActorKey boundActorKeyType

// actorFromContext returns the actor a bound credential fixed for this
// request, or "" when the request is local, legacy-token, or unauthenticated.
func actorFromContext(ctx context.Context) string {
	a, _ := ctx.Value(boundActorKey).(string)
	return a
}

func (h *HTTP) login(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		writeErr(w, &APIError{Code: "unauthorized", Message: "user auth not configured"})
		return
	}
	var body struct{ User, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &APIError{Code: "invalid_event", Message: "body must be JSON {user, password}"})
		return
	}
	actor, err := h.Auth.Store.VerifyLogin(r.Context(), body.User, body.Password)
	if err != nil {
		// Unknown user and wrong password are deliberately one error.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
			"code": "unauthorized", "message": "invalid credentials"}})
		return
	}
	// VerifyLogin does not return the user id; re-read it for the session row.
	var userID int64
	if err := h.Auth.Store.Pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE actor = $1`, actor).Scan(&userID); err != nil {
		writeErr(w, &APIError{Code: "internal", Message: err.Error()})
		return
	}
	raw, err := h.Auth.Store.CreateSession(r.Context(), userID)
	if err != nil {
		writeErr(w, &APIError{Code: "internal", Message: err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: raw, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"actor": actor})
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && h.Auth != nil {
		h.Auth.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// authState is public: the GUI reads it to choose the first-run setup view
// (no users yet) over the login view. It reveals only whether any user
// exists.
func (h *HTTP) authState(w http.ResponseWriter, r *http.Request) {
	firstRun := true
	if h.Auth != nil {
		exists, err := h.Auth.Store.UsersExist(r.Context())
		if err != nil {
			writeErr(w, &APIError{Code: "internal", Message: err.Error()})
			return
		}
		firstRun = !exists
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"first_run": firstRun})
}

// createUser is local-only once any user exists; on a fresh install (no
// users) it is open to remote callers as first-run bootstrap. The actor
// defaults to human:<username> on the first user.
func (h *HTTP) createUser(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		writeErr(w, &APIError{Code: "unauthorized", Message: "user auth not configured"})
		return
	}
	var body struct{ Username, Password, Actor string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &APIError{Code: "invalid_event", Message: "body must be JSON {username, password, actor}"})
		return
	}
	exists, err := h.Auth.Store.UsersExist(r.Context())
	if err != nil {
		writeErr(w, &APIError{Code: "internal", Message: err.Error()})
		return
	}
	if exists && !isLocal(r) {
		writeErr(w, &APIError{Code: "unauthorized", Message: "user administration is local-only"})
		return
	}
	if exists {
		if err := h.Auth.Store.CreateUser(r.Context(), body.Username, body.Password, body.Actor); err != nil {
			writeErr(w, &APIError{Code: "invalid_event", Message: err.Error()})
			return
		}
	} else {
		if err := h.Auth.Store.CreateFirstUser(r.Context(), body.Username, body.Password, body.Actor); err != nil {
			// includes ErrUsersExist from a racing bootstrap
			writeErr(w, &APIError{Code: "invalid_event", Message: err.Error()})
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"created"}`))
}

// createToken is local-only, like createUser. The raw token is in the
// response exactly once; only its hash is stored.
func (h *HTTP) createToken(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		writeErr(w, &APIError{Code: "unauthorized", Message: "token administration is local-only"})
		return
	}
	if h.Auth == nil {
		writeErr(w, &APIError{Code: "unauthorized", Message: "user auth not configured"})
		return
	}
	var body struct{ Actor, Label string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &APIError{Code: "invalid_event", Message: "body must be JSON {actor, label}"})
		return
	}
	raw, err := h.Auth.Store.CreateAPIToken(r.Context(), body.Actor, body.Label)
	if err != nil {
		writeErr(w, &APIError{Code: "invalid_event", Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"token": raw, "actor": body.Actor})
}

// resolveCredential authenticates a remote request. Bearer per-actor tokens
// win over the session cookie (an agent never carries both, but the order
// is deterministic). Returns the bound actor and whether a bound credential
// matched. SERVITOR_TOKEN matching is the caller's legacy path — handled in
// authed() and never bound.
func (h *HTTP) resolveCredential(ctx context.Context, r *http.Request) (string, bool) {
	if h.Auth == nil {
		return "", false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token != "" {
		if actor, ok := h.Auth.Store.TokenActor(ctx, token); ok {
			return actor, true
		}
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if actor, ok := h.Auth.Store.SessionActor(ctx, c.Value); ok {
			return actor, true
		}
	}
	return "", false
}
