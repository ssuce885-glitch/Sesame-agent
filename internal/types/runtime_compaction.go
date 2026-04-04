package types

import "time"

type ConversationCompactionKind string

const (
	ConversationCompactionKindMicro   ConversationCompactionKind = "micro"
	ConversationCompactionKindRolling ConversationCompactionKind = "rolling"
	ConversationCompactionKindFull    ConversationCompactionKind = "full"
)

type ProviderCacheKind string

const (
	ProviderCacheKindSession ProviderCacheKind = "session"
	ProviderCacheKindPrefix  ProviderCacheKind = "prefix"
)

type ProviderCacheStatus string

const (
	ProviderCacheStatusActive     ProviderCacheStatus = "active"
	ProviderCacheStatusSuperseded ProviderCacheStatus = "superseded"
	ProviderCacheStatusExpired    ProviderCacheStatus = "expired"
	ProviderCacheStatusFailed     ProviderCacheStatus = "failed"
	ProviderCacheStatusDeleted    ProviderCacheStatus = "deleted"
)

type ProviderCacheHead struct {
	SessionID         string    `json:"session_id"`
	Provider          string    `json:"provider"`
	CapabilityProfile string    `json:"capability_profile"`
	ActiveSessionRef  string    `json:"active_session_ref"`
	ActivePrefixRef   string    `json:"active_prefix_ref"`
	ActiveGeneration  int       `json:"active_generation"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ProviderCacheEntry struct {
	ID                string              `json:"id"`
	SessionID         string              `json:"session_id"`
	Provider          string              `json:"provider"`
	CapabilityProfile string              `json:"capability_profile"`
	CacheKind         ProviderCacheKind   `json:"cache_kind"`
	ExternalRef       string              `json:"external_ref"`
	ParentExternalRef string              `json:"parent_external_ref,omitempty"`
	Generation        int                 `json:"generation"`
	Status            ProviderCacheStatus `json:"status"`
	ExpiresAt         *time.Time          `json:"expires_at,omitempty"`
	LastUsedAt        *time.Time          `json:"last_used_at,omitempty"`
	MetadataJSON      string              `json:"metadata_json,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

type ConversationCompaction struct {
	ID              string                     `json:"id"`
	SessionID       string                     `json:"session_id"`
	Kind            ConversationCompactionKind `json:"kind"`
	Generation      int                        `json:"generation"`
	StartPosition   int                        `json:"start_position"`
	EndPosition     int                        `json:"end_position"`
	SummaryPayload  string                     `json:"summary_payload"`
	Reason          string                     `json:"reason"`
	ProviderProfile string                     `json:"provider_profile"`
	CreatedAt       time.Time                  `json:"created_at"`
}
