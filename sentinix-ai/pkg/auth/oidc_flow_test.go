package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeIdP is a minimal OIDC provider that records the PKCE challenge and nonce it was sent.
type fakeIdP struct {
	srv                      *httptest.Server
	key                      *rsa.PrivateKey
	challenge, nonce         string
	email                    string
	verified                 bool
	tokenNonceOverride       string
	sawVerifierMatchChalenge bool
}

func newIdP(t *testing.T) *fakeIdP {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	f := &fakeIdP{key: k, email: "a@corp.test", verified: true}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize",
			"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &k.PublicKey, KeyID: "k", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		f.sawVerifierMatchChalenge = base64.RawURLEncoding.EncodeToString(sum[:]) == f.challenge
		if !f.sawVerifierMatchChalenge {
			http.Error(w, "bad verifier", 400)
			return
		}
		nn := f.nonce
		if f.tokenNonceOverride != "" {
			nn = f.tokenNonceOverride
		}
		sg, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: k, KeyID: "k"}}, (&jose.SignerOptions{}).WithType("JWT"))
		raw, _ := jwt.Signed(sg).Claims(map[string]any{"iss": f.srv.URL, "aud": "cid", "sub": "u", "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "email": f.email, "email_verified": f.verified, "nonce": nn}).Serialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "token_type": "Bearer", "id_token": raw})
	})
	return f
}

// login drives Login then Callback; mutate may tamper with the callback request.
func (f *fakeIdP) login(t *testing.T, o *OIDC, mutate func(r *http.Request)) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	o.Login(w, httptest.NewRequest("GET", "/auth/login", nil))
	loc, _ := url.Parse(w.Header().Get("Location"))
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("nonce") == "" {
		t.Fatalf("authorize request lacks PKCE/nonce: %v", q)
	}
	f.challenge, f.nonce = q.Get("code_challenge"), q.Get("nonce")
	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+q.Get("state"), nil)
	for _, c := range w.Result().Cookies() {
		cb.AddCookie(c)
	}
	if mutate != nil {
		mutate(cb)
	}
	cw := httptest.NewRecorder()
	o.Callback(cw, cb)
	return cw
}

func TestOIDCFlowHardening(t *testing.T) {
	f := newIdP(t)
	var logged string
	o, err := NewOIDC(context.Background(), f.srv.URL, "cid", "sec", "http://app/auth/callback", Sessions{Key: []byte("k")}, func(e, n string) error { logged = e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if w := f.login(t, o, nil); w.Code != 302 || logged != "a@corp.test" {
		t.Fatalf("happy path: %d logged=%q", w.Code, logged)
	}
	f.tokenNonceOverride = "attacker-nonce"
	if w := f.login(t, o, nil); w.Code != 401 {
		t.Fatalf("wrong id_token nonce accepted: %d", w.Code)
	}
	f.tokenNonceOverride = ""
	if w := f.login(t, o, func(r *http.Request) { r.Header.Del("Cookie") }); w.Code != 400 {
		t.Fatalf("callback without state/pkce cookies accepted: %d", w.Code)
	}
	f.verified = false
	if w := f.login(t, o, nil); w.Code != 401 {
		t.Fatalf("unverified email accepted: %d", w.Code)
	}
	f.verified = true
	o.AllowedDomains = []string{"other.test"}
	if w := f.login(t, o, nil); w.Code != 403 {
		t.Fatalf("foreign domain accepted: %d", w.Code)
	}
	o.AllowedDomains = []string{"CORP.test"}
	if w := f.login(t, o, nil); w.Code != 302 {
		t.Fatalf("allowed domain (case-insensitive) rejected: %d", w.Code)
	}
}
