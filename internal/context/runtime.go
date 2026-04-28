package contextstate

import (
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

type Runtime struct {
	cacheExpirySeconds  int
	maxCompactionPasses int
}

func NewRuntime(cacheExpirySeconds, maxCompactionPasses int) *Runtime {
	return &Runtime{
		cacheExpirySeconds:  cacheExpirySeconds,
		maxCompactionPasses: maxCompactionPasses,
	}
}

func (r *Runtime) PrepareRequest(plan WorkingSet, head *types.ProviderCacheHead, caps model.ProviderCapabilities, userItem model.ConversationItem, instructions string) model.Request {
	recentRawItems := plan.RecentRawItems
	if len(recentRawItems) == 0 {
		recentRawItems = plan.RecentItems
	}

	hasUserItem := userItem.Kind != ""
	carryForwardItems := plan.CarryForwardItems
	if hasUserItem && userItem.Kind == model.ConversationItemUserMessage {
		carryForwardItems = stripFreshTurnProviderContinuationItems(carryForwardItems, caps)
		recentRawItems = stripFreshTurnProviderContinuationItems(recentRawItems, caps)
	}
	req := model.Request{
		UserMessage:  userItem.Text,
		Instructions: instructions,
		Items:        BuildReinjectedPromptItems(plan.Summaries, carryForwardItems, recentRawItems, userItem),
	}

	return r.applyCachePlan(req, plan, head, caps, userItem, hasUserItem)
}

func stripFreshTurnProviderContinuationItems(items []model.ConversationItem, caps model.ProviderCapabilities) []model.ConversationItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ConversationItem, 0, len(items))
	for _, item := range items {
		if item.Kind == model.ConversationItemAssistantThinking {
			continue
		}
		if caps.RequiresThinkingForToolContinuation && isToolContinuationItem(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func isToolContinuationItem(item model.ConversationItem) bool {
	return item.Kind == model.ConversationItemToolCall || item.Kind == model.ConversationItemToolResult
}

func BuildReinjectedPromptItems(bundle SummaryBundle, carryForward []model.ConversationItem, recentRaw []model.ConversationItem, userItem model.ConversationItem) []model.ConversationItem {
	hasUserItem := userItem.Kind != ""
	out := make([]model.ConversationItem, 0, 2+len(bundle.Rolling)+len(carryForward)+len(recentRaw)+map[bool]int{true: 1, false: 0}[hasUserItem])
	if bundle.ContextHeadSummary != nil {
		summary := cloneSummary(*bundle.ContextHeadSummary)
		out = append(out, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	if bundle.Boundary != nil {
		summary := cloneSummary(*bundle.Boundary)
		out = append(out, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	for _, summary := range bundle.Rolling {
		summary := cloneSummary(summary)
		out = append(out, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	out = append(out, cloneConversationItems(carryForward)...)
	out = append(out, cloneConversationItems(recentRaw)...)
	if hasUserItem {
		out = append(out, cloneConversationItem(userItem))
	}
	return out
}

func (r *Runtime) applyCachePlan(req model.Request, plan WorkingSet, head *types.ProviderCacheHead, caps model.ProviderCapabilities, userItem model.ConversationItem, hasUserItem bool) model.Request {
	if caps.Profile == model.CapabilityProfileNone {
		return req
	}

	prefixCompactionRequested := plan.CompactionApplied ||
		plan.Action.Kind == CompactionActionMicrocompact ||
		plan.Action.Kind == CompactionActionRolling ||
		plan.Action.Kind == CompactionActionArchive

	if caps.SupportsPrefixCache && shouldRotatePrefix(prefixCompactionRequested, head, r.maxCompactionPasses) {
		req.Cache = &model.CacheDirective{
			Mode:               model.CacheModePrefix,
			Store:              true,
			PreviousResponseID: previousPrefixResponseID(head),
			ExpireAt:           r.cacheExpiryUnix(),
		}
		return req
	}

	if !caps.SupportsSessionCache {
		return req
	}
	if plan.CompactionApplied && caps.RotatesSessionRef {
		req.Cache = &model.CacheDirective{
			Mode:     model.CacheModeSession,
			Store:    true,
			ExpireAt: r.cacheExpiryUnix(),
		}
		return req
	}
	if previous := previousSessionResponseID(head); previous != "" {
		if hasUserItem {
			req.Items = []model.ConversationItem{cloneConversationItem(userItem)}
		} else {
			req.Items = nil
		}
		req.Cache = &model.CacheDirective{
			Mode:               model.CacheModeSession,
			Store:              true,
			PreviousResponseID: previous,
			ExpireAt:           r.cacheExpiryUnix(),
		}
		return req
	}
	if head == nil {
		req.Cache = &model.CacheDirective{
			Mode:     model.CacheModeSession,
			Store:    true,
			ExpireAt: r.cacheExpiryUnix(),
		}
	}
	return req
}

func shouldRotatePrefix(compactionRequested bool, head *types.ProviderCacheHead, maxCompactionPasses int) bool {
	if !compactionRequested {
		return false
	}
	if head == nil || head.ActivePrefixRef == "" {
		return true
	}
	return maxCompactionPasses > 0 && head.ActiveGeneration >= maxCompactionPasses
}

func (r *Runtime) NextCacheHead(head *types.ProviderCacheHead, caps model.ProviderCapabilities, used *model.CacheDirective, meta *model.ResponseMetadata) *types.ProviderCacheHead {
	if meta == nil || meta.ResponseID == "" || used == nil || caps.Profile == model.CapabilityProfileNone {
		return head
	}

	if head == nil {
		head = &types.ProviderCacheHead{}
	}

	switch used.Mode {
	case model.CacheModePrefix:
		head.ActivePrefixRef = meta.ResponseID
		head.ActiveSessionRef = ""
		head.ActiveGeneration = 0
	case model.CacheModeSession:
		head.ActiveSessionRef = meta.ResponseID
		if head.ActiveGeneration > 0 {
			head.ActiveGeneration++
		} else {
			head.ActiveGeneration = 1
		}
	}

	head.UpdatedAt = time.Now().UTC()
	return head
}

func (r *Runtime) CacheDirectiveForHead(head *types.ProviderCacheHead, caps model.ProviderCapabilities, mode model.CacheMode) *model.CacheDirective {
	if head == nil || caps.Profile == model.CapabilityProfileNone {
		return nil
	}

	var previous string
	switch mode {
	case model.CacheModePrefix:
		previous = head.ActivePrefixRef
	case model.CacheModeSession:
		previous = head.ActiveSessionRef
		if previous == "" {
			previous = head.ActivePrefixRef
		}
	default:
		return nil
	}
	if previous == "" {
		return nil
	}

	return &model.CacheDirective{
		Mode:               mode,
		Store:              true,
		PreviousResponseID: previous,
		ExpireAt:           r.cacheExpiryUnix(),
	}
}

func (r *Runtime) cacheExpiryUnix() int64 {
	if r == nil || r.cacheExpirySeconds <= 0 {
		return 0
	}
	return time.Now().UTC().Add(time.Duration(r.cacheExpirySeconds) * time.Second).Unix()
}

func previousPrefixResponseID(head *types.ProviderCacheHead) string {
	if head == nil {
		return ""
	}
	return head.ActivePrefixRef
}

func previousSessionResponseID(head *types.ProviderCacheHead) string {
	if head == nil {
		return ""
	}
	if head.ActiveSessionRef != "" {
		return head.ActiveSessionRef
	}
	return head.ActivePrefixRef
}
