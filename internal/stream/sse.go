package stream

import (
	"encoding/json"
	"fmt"
	"net/http"

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
