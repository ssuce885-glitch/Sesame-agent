package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go-agent/internal/types"
)

func WriteSSE(w http.ResponseWriter, event types.Event) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Seq, event.Type, raw)
	return err
}

type KeepalivePayload struct {
	SessionID string    `json:"session_id"`
	LatestSeq int64     `json:"latest_seq"`
	Time      time.Time `json:"time"`
}

func WriteSSEKeepalive(w http.ResponseWriter, sessionID string, latestSeq int64, now time.Time) error {
	raw, err := json.Marshal(KeepalivePayload{
		SessionID: sessionID,
		LatestSeq: latestSeq,
		Time:      now.UTC(),
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: keepalive\ndata: %s\n\n", raw)
	return err
}
