package httpapi

import (
	"encoding/json"
	"net/http"
)

type StatusPayload struct {
	Status               string `json:"status"`
	DaemonID             string `json:"daemon_id,omitempty"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	ConfigFingerprint    string `json:"config_fingerprint,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

func registerStatusRoutes(mux *http.ServeMux, payload StatusPayload) {
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		payload.Status = "ok"
		_ = json.NewEncoder(w).Encode(payload)
	})
}
