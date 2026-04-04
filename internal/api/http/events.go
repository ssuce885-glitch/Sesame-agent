package httpapi

import (
	"net/http"
	"strconv"

	"go-agent/internal/stream"
)

func handleStreamEvents(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil || deps.Bus == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		afterSeq := int64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			afterSeq = parsed
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		sub := deps.Bus.Subscribe(sessionID)
		events, err := deps.Store.ListSessionEvents(r.Context(), sessionID, afterSeq)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		flush := func() {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}

		flush()

		lastSeq := afterSeq
		for _, event := range events {
			if err := stream.WriteSSE(w, event); err != nil {
				return
			}
			flush()
			if event.Seq > lastSeq {
				lastSeq = event.Seq
			}
		}

		for {
			select {
			case event, ok := <-sub:
				if !ok {
					return
				}
				if event.Seq <= lastSeq {
					continue
				}
				if err := stream.WriteSSE(w, event); err != nil {
					return
				}
				lastSeq = event.Seq
				flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
