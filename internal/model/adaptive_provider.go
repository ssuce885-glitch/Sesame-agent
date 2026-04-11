package model

import (
	"context"
	"fmt"
)

type AdaptiveProvider struct {
	resolved  ResolvedProviderConfig
	transport StreamingClient
}

func NewAdaptiveProvider(resolved ResolvedProviderConfig, transport StreamingClient) *AdaptiveProvider {
	return &AdaptiveProvider{
		resolved:  resolved,
		transport: transport,
	}
}

func (p *AdaptiveProvider) Capabilities() ProviderCapabilities {
	if p == nil || p.transport == nil {
		return ProviderCapabilities{}
	}
	return p.transport.Capabilities()
}

func (p *AdaptiveProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	if p == nil || p.transport == nil {
		return immediateStreamError(ctx, fmt.Errorf("adaptive provider is not configured"))
	}
	if p.resolved.Profile.StrictToolSequence {
		if err := ValidateConversationItems(req.Items); err != nil {
			return immediateStreamError(ctx, fmt.Errorf("%s transcript validation failed: %w", p.resolved.Profile.ID, err))
		}
	}
	return p.transport.Stream(ctx, req)
}

func (p *AdaptiveProvider) ResolvedConfig() ResolvedProviderConfig {
	if p == nil {
		return ResolvedProviderConfig{}
	}
	return p.resolved
}
