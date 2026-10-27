package server

import (
	"net/http"
)

func RegisterAdminRoutes() {
	http.HandleFunc("/api/admin/delete", func(w http.ResponseWriter, r *http.Request) {
		// Vulnerable: administrative delete endpoint has no authentication or authorization checks
		targetUser := r.URL.Query().Get("user_id")
		_ = targetUser
		w.Write([]byte(`{"status":"deleted"}`))
	})
}
