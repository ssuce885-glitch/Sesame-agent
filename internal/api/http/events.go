package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"go-agent/internal/stream"
)

var eventStreamKeepaliveInterval = 15 * time.Second

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
			if err != nil || parsed < 0 {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			afterSeq = parsed
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		sub, unsubscribe := deps.Bus.Subscribe(sessionID)
		defer unsubscribe()

		latestSeq, err := deps.Store.LatestSessionEventSeq(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if afterSeq > latestSeq {
			afterSeq = latestSeq
		}

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

		keepaliveTicker := time.NewTicker(eventStreamKeepaliveInterval)
		defer keepaliveTicker.Stop()

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

		if err := stream.WriteSSEKeepalive(w, sessionID, lastSeq, time.Now().UTC()); err != nil {
			return
		}
		flush()

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
			case <-keepaliveTicker.C:
				latestSeq, err := deps.Store.LatestSessionEventSeq(r.Context(), sessionID)
				if err != nil {
					latestSeq = lastSeq
				}
				if err := stream.WriteSSEKeepalive(w, sessionID, latestSeq, time.Now().UTC()); err != nil {
					return
				}
				flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
