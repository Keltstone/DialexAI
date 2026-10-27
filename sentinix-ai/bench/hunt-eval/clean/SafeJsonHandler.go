package clean

import (
	"encoding/json"
	"net/http"
)

type SafeUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func RegisterSafeJsonRoutes() {
	http.HandleFunc("/api/safe/import", func(w http.ResponseWriter, r *http.Request) {
		var req SafeUserRequest
		// Mitigation: strictly bounded and schema-typed JSON parsing
		r.Body = http.MaxBytesReader(w, r.Body, 1024*64)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Write([]byte(`{"status":"imported"}`))
	})
}
