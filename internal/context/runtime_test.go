package contextstate

import (
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestRuntimePrepareRequestRotatesPrefixWhenGenerationLimitIsReached(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{model.UserMessageItem("tail")},
			Summaries: []model.Summary{{
				RangeLabel: "turns 1-4",
			}},
		},
		Action: CompactionAction{Kind: CompactionActionRolling},
	}
	head := &types.ProviderCacheHead{
		SessionID:         "sess_1",
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActivePrefixRef:   "pref_prev",
		ActiveSessionRef:  "resp_prev",
		ActiveGeneration:  3,
		UpdatedAt:         time.Now().UTC(),
	}

	got := runtime.PrepareRequest(
		plan,
		head,
		model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
		},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if got.Cache == nil {
		t.Fatal("Cache = nil, want prefix rotation")
	}
	if got.Cache.Mode != model.CacheModePrefix {
		t.Fatalf("Cache.Mode = %q, want %q", got.Cache.Mode, model.CacheModePrefix)
	}
	if got.Cache.PreviousResponseID != "pref_prev" {
		t.Fatalf("Cache.PreviousResponseID = %q, want %q", got.Cache.PreviousResponseID, "pref_prev")
	}
	if len(got.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(got.Items))
	}
	if got.Items[0].Kind != model.ConversationItemSummary || got.Items[0].Summary == nil || got.Items[0].Summary.RangeLabel != "turns 1-4" {
		t.Fatalf("first item = %#v, want summary preserved", got.Items[0])
	}
	if got.Items[2].Kind != model.ConversationItemUserMessage || got.Items[2].Text != "follow up" {
		t.Fatalf("last item = %#v, want current user message", got.Items[2])
	}
}

func TestRuntimePrepareRequestFallsBackToLocalOnlyWhenProviderHasNoNativeCache(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{model.UserMessageItem("tail")},
			Summaries: []model.Summary{{
				RangeLabel: "turns 1-2",
			}},
		},
	}

	got := runtime.PrepareRequest(
		plan,
		&types.ProviderCacheHead{ActiveSessionRef: "resp_prev"},
		model.ProviderCapabilities{Profile: model.CapabilityProfileNone},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if got.Cache != nil {
		t.Fatalf("Cache = %#v, want nil local-only fallback", got.Cache)
	}
	if len(got.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(got.Items))
	}
	if got.Items[0].Kind != model.ConversationItemSummary || got.Items[0].Summary == nil || got.Items[0].Summary.RangeLabel != "turns 1-2" {
		t.Fatalf("first item = %#v, want summary preserved", got.Items[0])
	}
}

func TestRuntimePrepareRequestCreatesSessionCacheForFreshNativeSession(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{model.UserMessageItem("tail")},
		},
	}

	got := runtime.PrepareRequest(
		plan,
		nil,
		model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
		},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if got.Cache == nil {
		t.Fatal("Cache = nil, want fresh session cache")
	}
	if got.Cache.Mode != model.CacheModeSession {
		t.Fatalf("Cache.Mode = %q, want %q", got.Cache.Mode, model.CacheModeSession)
	}
	if got.Cache.Store != true {
		t.Fatal("Cache.Store = false, want true")
	}
	if got.Cache.PreviousResponseID != "" {
		t.Fatalf("Cache.PreviousResponseID = %q, want empty for fresh session", got.Cache.PreviousResponseID)
	}
}

func TestRuntimePrepareRequestUsesIncrementalUserItemWhenReusingSessionHead(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{
				model.UserMessageItem("old user"),
				{Kind: model.ConversationItemAssistantText, Text: "old assistant"},
			},
			Summaries: []model.Summary{{
				RangeLabel: "turns 1-2",
			}},
		},
	}

	got := runtime.PrepareRequest(
		plan,
		&types.ProviderCacheHead{
			ActiveSessionRef: "resp_prev",
		},
		model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
		},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if got.Cache == nil {
		t.Fatal("Cache = nil, want session continuation")
	}
	if got.Cache.Mode != model.CacheModeSession {
		t.Fatalf("Cache.Mode = %q, want %q", got.Cache.Mode, model.CacheModeSession)
	}
	if got.Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("Cache.PreviousResponseID = %q, want %q", got.Cache.PreviousResponseID, "resp_prev")
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 incremental user item", len(got.Items))
	}
	if got.Items[0].Kind != model.ConversationItemUserMessage || got.Items[0].Text != "follow up" {
		t.Fatalf("Items[0] = %#v, want follow-up user item", got.Items[0])
	}
}

func TestRuntimePrepareRequestStartsFreshSessionWhenCompactionWasApplied(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{
				model.UserMessageItem("old user"),
				{Kind: model.ConversationItemAssistantText, Text: "old assistant"},
			},
			Summaries: []model.Summary{{
				RangeLabel: "turns 1-2",
			}},
		},
		CompactionApplied: true,
	}

	got := runtime.PrepareRequest(
		plan,
		&types.ProviderCacheHead{
			ActiveSessionRef: "resp_prev",
		},
		model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			RotatesSessionRef:    true,
		},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if got.Cache == nil {
		t.Fatal("Cache = nil, want fresh session cache after compaction")
	}
	if got.Cache.Mode != model.CacheModeSession {
		t.Fatalf("Cache.Mode = %q, want %q", got.Cache.Mode, model.CacheModeSession)
	}
	if got.Cache.PreviousResponseID != "" {
		t.Fatalf("Cache.PreviousResponseID = %q, want empty for refreshed session", got.Cache.PreviousResponseID)
	}
	if len(got.Items) != 4 {
		t.Fatalf("len(Items) = %d, want 4 full items after compaction", len(got.Items))
	}
	if got.Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("Items[0] = %#v, want summary preserved", got.Items[0])
	}
	if got.Items[3].Kind != model.ConversationItemUserMessage || got.Items[3].Text != "follow up" {
		t.Fatalf("Items[3] = %#v, want current user message", got.Items[3])
	}
}

func TestRuntimePrepareRequestPrefersPromptItemsWhenPresent(t *testing.T) {
	runtime := NewRuntime(86400, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: []model.ConversationItem{
				model.UserMessageItem("recent only"),
			},
			PromptItems: []model.ConversationItem{
				{
					Kind: model.ConversationItemSummary,
					Summary: &model.Summary{
						RangeLabel: "historical compacted tool results",
					},
				},
				model.UserMessageItem("recent only"),
			},
		},
	}

	got := runtime.PrepareRequest(
		plan,
		nil,
		model.ProviderCapabilities{Profile: model.CapabilityProfileNone},
		model.UserMessageItem("follow up"),
		"system rules",
	)

	if len(got.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3 prompt items + user", len(got.Items))
	}
	if got.Items[0].Kind != model.ConversationItemSummary || got.Items[0].Summary == nil || got.Items[0].Summary.RangeLabel != "historical compacted tool results" {
		t.Fatalf("Items[0] = %#v, want prompt summary item", got.Items[0])
	}
}
