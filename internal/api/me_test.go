package api

// /api/me and POST /api/password (user-details-ui ticket).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loginCookie(t *testing.T, h *HTTP, user, password string) string {
	t.Helper()
	rec := login(t, h, user, password)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "servitor_session" {
			return c.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func TestMeEndpoint(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "pw", "human:preston")
	cookie := loginCookie(t, h, "preston", "pw")

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"user":"preston"`, `"actor":"human:preston"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("me missing %s: %s", want, rec.Body.String())
		}
	}

	req = httptest.NewRequest("GET", "/api/me", nil)
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("me without session: %d, want 401", rec.Code)
	}
}

func TestChangePassword(t *testing.T) {
	h, st := authedHTTP(t)
	createTestUser(t, st, "preston", "old-pw", "human:preston")
	cookie := loginCookie(t, h, "preston", "old-pw")

	// wrong current password: refused, nothing changes
	rec := changePw(t, h, cookie, "wrong", "new-pw")
	if rec.Code != 401 {
		t.Fatalf("wrong current: %d, want 401", rec.Code)
	}
	if rec := login(t, h, "preston", "new-pw"); rec.Code == 200 {
		t.Error("password changed without correct current")
	}

	// correct change: current session survives
	rec = changePw(t, h, cookie, "old-pw", "new-pw")
	if rec.Code != 200 {
		t.Fatalf("change: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("current session must survive: %d", rec.Code)
	}

	// old password dead, new one works
	if rec := login(t, h, "preston", "old-pw"); rec.Code != 401 {
		t.Errorf("old password still valid: %d", rec.Code)
	}
	if rec := login(t, h, "preston", "new-pw"); rec.Code != 200 {
		t.Errorf("new password rejected: %d", rec.Code)
	}
	_ = st
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	h, _ := authedHTTP(t)
	createTestUser(t, h.Auth.Store, "preston", "old-pw", "human:preston")
	keep := loginCookie(t, h, "preston", "old-pw")
	other := loginCookie(t, h, "preston", "old-pw")

	if rec := changePw(t, h, keep, "old-pw", "new-pw"); rec.Code != 200 {
		t.Fatalf("change: %d", rec.Code)
	}

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: keep})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("changing session revoked: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: other})
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("other session still alive: %d, want 401", rec.Code)
	}
}

func changePw(t *testing.T, h *HTTP, cookie, current, next string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"current":"` + current + `","new":"` + next + `"}`
	req := httptest.NewRequest("POST", "/api/password", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "servitor_session", Value: cookie})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}
