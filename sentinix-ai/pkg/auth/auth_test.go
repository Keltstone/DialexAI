package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRoles(t *testing.T) {
	if Can(Developer, "triage") || Can(Auditor, "manage") || Can(Admin, "triage") || !Can(Reviewer, "approve") || !Can(Owner, "owner") {
		t.Fatal("role matrix wrong")
	}
}

func roundTrip(s Sessions, mutate func(*http.Cookie)) (string, bool) {
	w := httptest.NewRecorder()
	s.Issue(w, "a@example.test", time.Hour)
	c := w.Result().Cookies()[0]
	mutate(c)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(c)
	return s.Email(r)
}

func TestSession(t *testing.T) {
	s := Sessions{Key: []byte("k")}
	if e, ok := roundTrip(s, func(*http.Cookie) {}); !ok || e != "a@example.test" {
		t.Fatal("valid session rejected")
	}
	if _, ok := roundTrip(s, func(c *http.Cookie) { c.Value = "x" + c.Value }); ok {
		t.Fatal("tampered session accepted")
	}
	if _, ok := roundTrip(Sessions{Key: []byte("other")}, func(c *http.Cookie) {}); !ok {
		t.Fatal("sanity")
	}
	w := httptest.NewRecorder()
	s.Issue(w, "a@example.test", -time.Hour)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(w.Result().Cookies()[0])
	if _, ok := s.Email(r); ok {
		t.Fatal("expired session accepted")
	}
}
