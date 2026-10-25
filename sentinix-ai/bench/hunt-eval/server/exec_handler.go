package server

import (
	"net/http"
	"os/exec"
)

func RegisterExecRoutes() {
	http.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("host")
		// Vulnerable: command string concatenated and executed in shell
		cmd := exec.Command("sh", "-c", "ping -c 1 "+host)
		out, err := cmd.CombinedOutput()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(out)
	})
}
