package clean

import (
	"net/http"
	"os/exec"
)

func RegisterSafeExecRoutes() {
	http.HandleFunc("/api/safe/ping", func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("host")
		// Mitigation: no shell invocation; arguments passed strictly as slice elements
		cmd := exec.Command("ping", "-c", "1", host)
		out, err := cmd.CombinedOutput()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(out)
	})
}
