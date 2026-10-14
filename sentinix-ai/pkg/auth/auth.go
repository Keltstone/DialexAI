// Package auth: roles, signed session cookies and OIDC login.
// SAML is supported by putting an identity broker (Keycloak, Authentik) in front and speaking OIDC to it.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	Owner     = "owner"
	Admin     = "admin"
	Reviewer  = "reviewer"
	Developer = "developer"
	Auditor   = "auditor"
)

var perms = map[string][]string{
	"read":    {Owner, Admin, Reviewer, Developer, Auditor},
	"triage":  {Owner, Reviewer},
	"scan":    {Owner, Reviewer},
	"approve": {Owner, Reviewer},
	"audit":   {Owner, Admin, Reviewer, Auditor},
	"manage":  {Owner, Admin}, // projects, integrations, packs, users, tokens
	"owner":   {Owner},        // SSO setup, executable plugins
}

func Can(role, perm string) bool {
	for _, r := range perms[perm] {
		if r == role {
			return true
		}
	}
	return false
}

func ValidRole(r string) bool {
	for _, x := range perms["read"] {
		if x == r {
			return true
		}
	}
	return false
}

// Sessions are HMAC-signed cookies: base64(json).hex(mac). No server-side state.
type Sessions struct {
	Key    []byte
	Secure bool // set Secure on the cookie (true behind HTTPS)
}

const cookie = "sx_session"

type claims struct {
	Email string `json:"e"`
	Exp   int64  `json:"x"`
}

func (s Sessions) sign(p string) string {
	m := hmac.New(sha256.New, s.Key)
	m.Write([]byte(p))
	return hex.EncodeToString(m.Sum(nil))
}

func (s Sessions) Issue(w http.ResponseWriter, email string, ttl time.Duration) {
	b, _ := json.Marshal(claims{email, time.Now().Add(ttl).Unix()})
	p := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: p + "." + s.sign(p), Path: "/", HttpOnly: true,
		Secure: s.Secure, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
}

func (s Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

func (s Sessions) Email(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookie)
	if err != nil {
		return "", false
	}
	p, mac, ok := strings.Cut(c.Value, ".")
	if !ok || subtle.ConstantTimeCompare([]byte(mac), []byte(s.sign(p))) != 1 {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(p)
	var cl claims
	if err != nil || json.Unmarshal(b, &cl) != nil || cl.Exp < time.Now().Unix() || cl.Email == "" {
		return "", false
	}
	return cl.Email, true
}

// OIDC does the authorization-code login. OnLogin maps the verified identity to a stored user.
type OIDC struct {
	cfg      oauth2.Config
	verifier *oidc.IDTokenVerifier
	Sess     Sessions
	OnLogin  func(email, name string) error
}

func NewOIDC(ctx context.Context, issuer, clientID, secret, redirect string, sess Sessions, on func(email, name string) error) (*OIDC, error) {
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return &OIDC{
		cfg: oauth2.Config{ClientID: clientID, ClientSecret: secret, RedirectURL: redirect, Endpoint: p.Endpoint(),
			Scopes: []string{oidc.ScopeOpenID, "email", "profile"}},
		verifier: p.Verifier(&oidc.Config{ClientID: clientID}), Sess: sess, OnLogin: on}, nil
}

func nonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (o *OIDC) Login(w http.ResponseWriter, r *http.Request) {
	st := nonce()
	http.SetCookie(w, &http.Cookie{Name: "sx_state", Value: st, Path: "/auth", HttpOnly: true, Secure: o.Sess.Secure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, o.cfg.AuthCodeURL(st), http.StatusFound)
}

func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("sx_state")
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		http.Error(w, "bad state", http.StatusBadRequest)
		return
	}
	tok, err := o.cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := o.verifier.Verify(r.Context(), raw)
	var cl struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err != nil || idt.Claims(&cl) != nil || cl.Email == "" || !cl.EmailVerified {
		http.Error(w, "login failed", http.StatusUnauthorized)
		return
	}
	if err := o.OnLogin(strings.ToLower(cl.Email), cl.Name); err != nil {
		http.Error(w, fmt.Sprint("login failed"), http.StatusInternalServerError)
		return
	}
	o.Sess.Issue(w, strings.ToLower(cl.Email), 12*time.Hour)
	http.Redirect(w, r, "/", http.StatusFound)
}
