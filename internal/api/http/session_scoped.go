package httpapi

import (
	"net/http"
	"strings"
)

func registerSessionScopedRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.NotFound(w, r)
			return
		}

		sessionID := parts[0]
		switch parts[1] {
		case "turns":
			handleSubmitTurn(deps, sessionID)(w, r)
		case "events":
			handleStreamEvents(deps, sessionID)(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
