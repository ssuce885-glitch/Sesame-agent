package httpapi

import "net/http"

func registerMemoryRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/memory/candidates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
}
