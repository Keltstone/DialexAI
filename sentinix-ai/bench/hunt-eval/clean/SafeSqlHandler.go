package clean

import (
	"database/sql"
	"net/http"
)

func RegisterSafeSqlRoutes(db *sql.DB) {
	http.HandleFunc("/api/safe/user", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		// Mitigation: parameterized prepared query using $1 placeholder
		rows, err := db.Query("SELECT id, name, email FROM users WHERE id = $1", id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
	})
}
