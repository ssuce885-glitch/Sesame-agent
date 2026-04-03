package httpapi

import "net/http"

func registerPermissionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/permissions/decide", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
