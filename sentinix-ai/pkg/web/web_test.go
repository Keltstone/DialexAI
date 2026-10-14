package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServesDashboardWithStrictCSP(t *testing.T) {
	for _, p := range []string{"/", "/app.js", "/style.css"} {
		w := httptest.NewRecorder()
		Handler().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
			t.Fatalf("%s: %d %v", p, w.Code, w.Header())
		}
	}
}

func TestNoInnerHTMLOrInlineScript(t *testing.T) {
	b, _ := assets.ReadFile("static/app.js")
	i, _ := assets.ReadFile("static/index.html")
	if strings.Contains(string(b), ".innerHTML") || strings.Contains(string(i), "<script>") {
		t.Fatal("dashboard must not use innerHTML or inline scripts")
	}
}
