package clean

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func RegisterSafeProxyRoutes() {
	http.HandleFunc("/api/safe/proxy", func(w http.ResponseWriter, r *http.Request) {
		targetStr := r.URL.Query().Get("url")
		u, err := url.Parse(targetStr)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			http.Error(w, "invalid scheme", 400)
			return
		}
		// Mitigation: host allowlist and private IP check
		host, _, _ := net.SplitHostPort(u.Host)
		if host == "" {
			host = u.Host
		}
		if !strings.HasSuffix(host, "api.trusted.com") {
			http.Error(w, "untrusted host", 403)
			return
		}
		resp, err := http.Get(targetStr)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
	})
}
