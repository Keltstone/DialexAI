package server

import (
	"encoding/gob"
	"net/http"
)

type SessionPayload struct {
	User string
	Data any
}

func RegisterDeserRoutes() {
	http.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		var payload SessionPayload
		// Vulnerable: deserializes untrusted binary stream directly without schema validation
		dec := gob.NewDecoder(r.Body)
		if err := dec.Decode(&payload); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Write([]byte(`{"status":"imported"}`))
	})
}
