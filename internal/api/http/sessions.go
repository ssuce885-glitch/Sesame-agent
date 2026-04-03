package httpapi

import "net/http"

func registerSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sess_test","workspace_root":"D:/work/demo"}`))
	})
}
