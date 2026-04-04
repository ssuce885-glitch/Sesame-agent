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
	fullItems := make([]model.ConversationItem, 0, len(plan.Summaries)+len(plan.RecentItems)+1)
	for _, summary := range plan.Summaries {
		summary := summary
		fullItems = append(fullItems, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	fullItems = append(fullItems, cloneConversationItems(plan.RecentItems)...)
	fullItems = append(fullItems, cloneConversationItem(userItem))

	req := model.Request{
		UserMessage:  userItem.Text,
		Instructions: instructions,
		Items:        fullItems,
	}

	if caps.Profile == model.CapabilityProfileNone {
		return req
	}

	if caps.SupportsPrefixCache && shouldRotatePrefix(plan.Action, head, r.maxCompactionPasses) {
		req.Cache = &model.CacheDirective{
			Mode:               model.CacheModePrefix,
			Store:              true,
			PreviousResponseID: previousPrefixResponseID(head),
			ExpireAt:           r.cacheExpiryUnix(),
		}
		return req
	}

	if caps.SupportsSessionCache {
		if previous := previousSessionResponseID(head); previous != "" {
			req.Items = []model.ConversationItem{cloneConversationItem(userItem)}
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
				Mode:               model.CacheModeSession,
				Store:              true,
				ExpireAt:           r.cacheExpiryUnix(),
			}
		}
	}

	return req
}

func shouldRotatePrefix(action CompactionAction, head *types.ProviderCacheHead, maxCompactionPasses int) bool {
	if action.Kind != CompactionActionMicrocompact && action.Kind != CompactionActionRolling {
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
