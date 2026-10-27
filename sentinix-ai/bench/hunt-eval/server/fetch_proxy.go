package server

import (
	"io"
	"net/http"
)

func RegisterProxyRoutes() {
	http.HandleFunc("/api/proxy", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("url")
		// Vulnerable: performs HTTP fetch to unvalidated target URL without restricting internal IP ranges
		resp, err := http.Get(target)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
	})
}
