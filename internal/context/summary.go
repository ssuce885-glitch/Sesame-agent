package contextstate

import "go-agent/internal/model"

type Summary = model.Summary

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type WorkingContext struct {
	RecentItems    []model.ConversationItem `json:"recent_items"`
	Summaries      []model.Summary          `json:"summaries"`
	MemoryRefs     []string                 `json:"memory_refs"`
	RecentMessages []Message                `json:"recent_messages,omitempty"`
}
