package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sentinix/pkg/auth"
)

func TestSecurityHeadersEverywhere(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/healthz", "/api/me"} {
		h := e.call("", "", "GET", p, "").Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s missing hardening headers: %v", p, h)
		}
	}
}

func TestRevokedSessionDiesAndRelogWorks(t *testing.T) {
	e := setup(t)
	e.st.UpsertUser("victim@example.test", "v", auth.Developer)
	issue := func() *http.Cookie {
		w := httptest.NewRecorder()
		e.app.Sess.Issue(w, "victim@example.test", time.Hour)
		return w.Result().Cookies()[0]
	}
	me := func(c *http.Cookie) int {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w.Code
	}
	old := issue()
	if me(old) != 200 {
		t.Fatal("fresh session rejected")
	}
	e.ok(e.call("admin@example.test", auth.Admin, "POST", "/api/users/revoke", `{"Email":"victim@example.test"}`), 204)
	if me(old) != 401 {
		t.Fatal("revoked session still valid")
	}
	time.Sleep(5 * time.Millisecond)
	if me(issue()) != 200 {
		t.Fatal("re-login after revoke rejected")
	}
	// logout needs the CSRF header
	r := httptest.NewRequest("POST", "/auth/logout", nil)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-site logout allowed: %d", w.Code)
	}
}
