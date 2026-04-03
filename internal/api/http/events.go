package httpapi

import "net/http"

func registerEventRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}
