package clean

import (
	"net/http"
)

func RegisterSafeAdminRoutes() {
	http.HandleFunc("/api/safe/admin/delete", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		// Mitigation: role-based authentication check
		if token != "Bearer admin-secret-token" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"status":"deleted"}`))
	})
}
