package httpapi

import "net/http"

func registerSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
}
