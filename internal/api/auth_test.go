package api

// Credential auth tests. Risk-ordered: the actor matrix first, because it
// proves the existing localhost CLI human-gate flow survives before any
// new credential machinery is trusted.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/store"
)

func authedHTTP(t *testing.T) (*HTTP, *store.Store) {
	t.Helper()
	st := storeForTest(t)
	h := NewHTTP(NewStoreService(st))
	h.Auth = &Authenticator{Store: st}
	return h, st
}

func createTestUser(t *testing.T, st *store.Store, username, password, actor string) {
	t.Helper()
	if err := st.CreateUser(context.Background(), username, password, actor); err != nil {
		t.Fatal(err)
	}
}

func login(t *testing.T, h *HTTP, user, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"user":"` + user + `","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func tokenFor(t *testing.T, h *HTTP, actor string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/token", strings.NewReader(`{"actor":"`+actor+`"}`))
	req.RemoteAddr = "127.0.0.1:1234" // local-only endpoint
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("token create: %d %s", rec.Code, rec.Body.String())
	}
	var out struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Token
}

func writeEvent(t *testing.T, h *HTTP, actorClaim, cookie, bearer string, remote bool) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"kind":"note","actor":"agent:test","payload":{"text":"x"},"ticket":"` + ticketULID + `"}`
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	if remote {
		req.RemoteAddr = "192.168.1.90:55555"
	} else {
		req.RemoteAddr = "127.0.0.1:55555"
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
	}
	if actorClaim != "" {
		req.Header.Set("X-Servitor-Actor", actorClaim)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

// lastActor reads the actor on the newest event.
func lastActor(t *testing.T, st *store.Store) string {
	t.Helper()
	evs, err := st.Events(context.Background(), store.LedgerFilter{Limit: 1})
	if err != nil || len(evs) == 0 {
		t.Fatalf("events: %v %v", evs, err)
	}
	return evs[0].Actor
}

var ticketULID string

func newTicket(t *testing.T, h *HTTP) {
	t.Helper()
	ticketULID = store.NewULID()
	body := `{"kind":"ticket.create","actor":"agent:test","ticket":"` + ticketULID + `","payload":{"slug":"auth-mx-` + ticketULID[:8] + `","title":"auth matrix"}}`
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("ticket create: %d %s", rec.Code, rec.Body.String())
	}
}

// TestActorMatrix is the risk-first test: for every (credential type x
// claimed X-Servitor-Actor header) pair, the event's actor column says who
// the credential bound, or the claim stands for local/legacy callers.
func TestActorMatrix(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "correct horse", "human:preston")
	newTicket(t, h)
	sess := login(t, h, "preston", "correct horse")
	if sess.Code != 200 {
		t.Fatalf("login: %d %s", sess.Code, sess.Body.String())
	}
	var cookie string
	for _, c := range sess.Result().Cookies() {
		if c.Name == "servitor_session" {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("no session cookie set")
	}
	tok := tokenFor(t, h, "agent:hermes")

	cases := []struct {
		name       string
		remote     bool
		cookie     string
		bearer     string
		claim      string
		want       string
		wantStatus int
	}{
		// local: header attestation survives unchanged (criterion 5)
		{"local claim honored", false, "", "", "human:preston", "human:preston", 201},
		{"local default actor", false, "", "", "", "agent:test", 201},
		// legacy bootstrap token from remote: header still attests — that
		// is what the bootstrap path is for
		{"remote bootstrap token, claim stands", true, "", "bootstrap", "human:preston", "human:preston", 201},
		{"remote bootstrap token, no claim", true, "", "bootstrap", "", "agent:test", 201},
		// per-actor token: credential wins over any claim (criterion 4)
		{"token claim overridden", true, "", tok, "human:preston", "agent:hermes", 201},
		{"token no claim", true, "", tok, "", "agent:hermes", 201},
		// session: credential wins over any claim
		{"session claim overridden", true, cookie, "", "agent:hermes", "human:preston", 201},
		{"session no claim", true, cookie, "", "", "human:preston", 201},
		// session + token both: token wins (deterministic order)
		{"token beats session", true, cookie, tok, "", "agent:hermes", 201},
		// unauthenticated remote: 401 regardless of claim
		{"remote unauthenticated", true, "", "", "human:preston", "", 401},
		{"remote junk token", true, "", "junk", "", "", 401},
	}
	// the bootstrap-token case needs the legacy token actually set
	h2 := *h
	h2.Token = "bootstrap"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := h
			if tc.bearer == "bootstrap" {
				target = &h2
			}
			rec := writeEvent(t, target, tc.claim, tc.cookie, tc.bearer, tc.remote)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if tc.want != "" && lastActor(t, st) != tc.want {
				t.Errorf("actor = %q, want %q", lastActor(t, st), tc.want)
			}
		})
	}
}

func TestLoginBadCredentials(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "bekah", "pw-bekah", "human:bekah")
	for _, tc := range []struct{ user, pw string }{
		{"bekah", "wrong"},
		{"nobody", "pw-bekah"},
		{"bekah", "pw-bekah"}, // control: right password — 200, cookie set
	} {
		rec := login(t, h, tc.user, tc.pw)
		ok := tc.user == "bekah" && tc.pw == "pw-bekah"
		if ok && rec.Code != 200 {
			t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
		}
		if ok {
			var cookies []*http.Cookie
			cookies = rec.Result().Cookies()
			found := false
			for _, c := range cookies {
				if c.Name == "servitor_session" && c.HttpOnly {
					found = true
				}
			}
			if !found {
				t.Error("session cookie not HttpOnly or missing")
			}
			continue
		}
		if rec.Code != 401 {
			t.Errorf("%s/%s: status %d, want 401", tc.user, tc.pw, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "unknown") || strings.Contains(rec.Body.String(), "no such") {
			t.Errorf("error must not distinguish unknown user: %s", rec.Body.String())
		}
	}
}

// TestSessionSurvivesRestart proves sessions live in the database: a new
// Authenticator (as a restarted daemon would construct) still honors the
// cookie.
func TestSessionSurvivesRestart(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "pw", "human:preston")
	newTicket(t, h)
	rec := login(t, h, "preston", "pw")
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "servitor_session" {
			cookie = c.Value
		}
	}

	restarted := NewHTTP(NewStoreService(st))
	restarted.Auth = &Authenticator{Store: st}
	out := writeEvent(t, restarted, "", cookie, "", true)
	if out.Code != 201 {
		t.Fatalf("post-restart request: %d %s", out.Code, out.Body.String())
	}
	if got := lastActor(t, st); got != "human:preston" {
		t.Errorf("actor = %q, want human:preston", got)
	}
}

// TestLocalHeaderUnchanged pins the existing localhost CLI human-gate flow
// against regressions from the actor binding (X-Servitor-Actor over
// loopback, no Auth configured at all — the pre-user-auth deployment).
func TestLocalHeaderUnchanged(t *testing.T) {
	st := storeForTest(t)
	legacy := NewHTTP(NewStoreService(st)) // no Auth, no Token
	id := mustCreate(t, NewStoreService(st), "legacy-header")
	body := `{"ticket":"` + id + `","kind":"gate","actor":"agent:test","payload":{"gate":"contract_approved"}}`
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	req.Header.Set("X-Servitor-Actor", "human:preston")
	rec := httptest.NewRecorder()
	legacy.Routes().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminEndpointsLocalOnly: user/token creation is refused from a
// network caller even when authenticated.
func TestAdminEndpointsLocalOnly(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "pw", "human:preston")
	newTicket(t, h)
	rec := login(t, h, "preston", "pw")
	var cookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "servitor_session" {
			cookie = c.Value
		}
	}
	for _, path := range []string{"/api/user", "/api/token"} {
		req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(`{}`)))
		req.RemoteAddr = "192.168.1.90:55555"
		req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
		out := httptest.NewRecorder()
		h.Routes().ServeHTTP(out, req)
		if out.Code != 401 {
			t.Errorf("%s from remote: %d, want 401", path, out.Code)
		}
	}
}

// TestNoTokenStillGuards: with user auth on and SERVITOR_TOKEN unset, a
// remote request without a credential is 401 (regression: the old guard
// returned early when Token was empty, leaving the API open), while
// loopback stays exempt.
func TestNoTokenStillGuards(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "pw", "human:preston")
	newTicket(t, h)

	remote := httptest.NewRequest("GET", "/api/board", nil)
	remote.RemoteAddr = "192.168.1.90:55555"
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, remote)
	if rec.Code != 401 {
		t.Errorf("remote no token, no SERVITOR_TOKEN: %d, want 401", rec.Code)
	}
	local := httptest.NewRequest("GET", "/api/board", nil)
	local.RemoteAddr = "127.0.0.1:55555"
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, local)
	if rec.Code != 200 {
		t.Errorf("loopback: %d, want 200", rec.Code)
	}
}

// TestFirstRunBootstrap: with zero users, a remote caller can create the
// first user and log in; once a user exists, remote creation is refused
// and auth-state flips.
func TestFirstRunBootstrap(t *testing.T) {
	h, st := authedHTTP(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth-state", nil)
	req.RemoteAddr = "192.168.1.90:55555"
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"first_run":true`) {
		t.Fatalf("auth-state: %d %s", rec.Code, rec.Body.String())
	}

	body := `{"username":"preston","password":"pw","actor":""}`
	req = httptest.NewRequest("POST", "/api/user", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.90:55555"
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("first-run create: %d %s", rec.Code, rec.Body.String())
	}
	// the created user's actor defaulted to human:preston and can log in
	rec = login(t, h, "preston", "pw")
	if rec.Code != 200 {
		t.Fatalf("login after bootstrap: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/auth-state", nil)
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), `"first_run":true`) {
		t.Error("auth-state still first_run after bootstrap")
	}

	// remote creation now refused, even authenticated
	newTicket(t, h)
	sess := login(t, h, "preston", "pw")
	var cookie string
	for _, c := range sess.Result().Cookies() {
		if c.Name == "servitor_session" {
			cookie = c.Value
		}
	}
	req = httptest.NewRequest("POST", "/api/user", strings.NewReader(`{"username":"eve","password":"x","actor":""}`))
	req.RemoteAddr = "192.168.1.90:55555"
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("remote second create: %d, want 401", rec.Code)
	}
	// same refusal, unauthenticated — and the status must be 401, not the
	// default 500 (regression: unauthorized was missing from HTTPStatus)
	req = httptest.NewRequest("POST", "/api/user", strings.NewReader(`{"username":"eve","password":"x","actor":""}`))
	req.RemoteAddr = "192.168.1.90:55555"
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("remote unauth second create: %d, want 401", rec.Code)
	}
	// and a racing local-only double bootstrap loses too
	if err := st.CreateFirstUser(context.Background(), "eve", "x", ""); err == nil {
		t.Error("CreateFirstUser on non-empty table must fail")
	}
}

// TestTokenActorBinding: the token's actor lands on the event, and the
// legacy SERVITOR_TOKEN still authenticates as it did (criterion 7).
func TestTokenActorBindingAndLegacyToken(t *testing.T) {
	h, st := authedHTTP(t)
	h.Token = "bootstrap-token"
	createTestUser(t, st, "preston", "pw", "human:preston")
	newTicket(t, h)
	tok := tokenFor(t, h, "agent:claude")

	rec := writeEvent(t, h, "", "", tok, true)
	if rec.Code != 201 {
		t.Fatalf("token write: %d %s", rec.Code, rec.Body.String())
	}
	if got := lastActor(t, st); got != "agent:claude" {
		t.Errorf("actor = %q, want agent:claude", got)
	}

	rec = writeEvent(t, h, "", "", "bootstrap-token", true)
	if rec.Code != 201 {
		t.Fatalf("legacy token write: %d %s", rec.Code, rec.Body.String())
	}
}
