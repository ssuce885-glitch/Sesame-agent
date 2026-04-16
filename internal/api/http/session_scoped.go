package httpapi

import (
	"net/http"
	"strings"
)

func registerSessionScopedRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		parts := strings.Split(rest, "/")
		if len(parts) == 3 && parts[0] != "" && parts[1] == "files" && parts[2] == "content" {
			handleGetSessionFileContent(deps, parts[0])(w, r)
			return
		}
		if len(parts) == 1 && parts[0] != "" {
			if r.Method == http.MethodPatch {
				handlePatchSession(deps, parts[0])(w, r)
				return
			}
			if r.Method == http.MethodDelete {
				handleDeleteSession(deps, parts[0])(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.NotFound(w, r)
			return
		}

		sessionID := parts[0]
		switch parts[1] {
		case "turns":
			handleSubmitTurn(deps, sessionID)(w, r)
		case "interrupt":
			handleInterruptTurn(deps, sessionID)(w, r)
		case "events":
			handleStreamEvents(deps, sessionID)(w, r)
		case "select":
			handleSelectSession(deps, sessionID)(w, r)
		case "timeline":
			handleGetTimeline(deps, sessionID)(w, r)
		case "mailbox":
			handleGetReportMailbox(deps, sessionID)(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
