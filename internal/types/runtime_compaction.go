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
	ContextHeadID   string                     `json:"context_head_id,omitempty"`
	Kind            ConversationCompactionKind `json:"kind"`
	Generation      int                        `json:"generation"`
	StartItemID     int64                      `json:"start_item_id,omitempty"`
	EndItemID       int64                      `json:"end_item_id,omitempty"`
	StartPosition   int                        `json:"start_position"`
	EndPosition     int                        `json:"end_position"`
	SummaryPayload  string                     `json:"summary_payload"`
	MetadataJSON    string                     `json:"metadata_json,omitempty"`
	Reason          string                     `json:"reason"`
	ProviderProfile string                     `json:"provider_profile"`
	CreatedAt       time.Time                  `json:"created_at"`
}

type CompactionBoundaryMetadata struct {
	Version               int    `json:"version"`
	PromptLayoutVersion   int    `json:"prompt_layout_version"`
	Generation            int    `json:"generation"`
	CompactedStart        int    `json:"compacted_start"`
	CompactedEnd          int    `json:"compacted_end"`
	PreservedRecentStart  int    `json:"preserved_recent_start"`
	HeadMemoryUpTo        int64  `json:"head_memory_up_to,omitempty"`
	SourceItemCount       int    `json:"source_item_count"`
	Reason                string `json:"reason"`
	ProviderProfile       string `json:"provider_profile"`
	HasRecentMicrocompact bool   `json:"has_recent_microcompact"`
}
