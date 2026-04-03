package httpapi

import "net/http"

func registerTurnRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/turns", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}
