package server

import (
	"database/sql"
	"fmt"
	"net/http"
)

func RegisterUserRoutes(db *sql.DB) {
	http.HandleFunc("/api/user", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		// Vulnerable: user input directly concatenated into raw SQL string
		query := fmt.Sprintf("SELECT id, name, email FROM users WHERE id = '%s'", id)
		rows, err := db.Query(query)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
	})
}
