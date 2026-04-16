package sessionbinding

import (
	"context"
	"strings"
)

const (
	HeaderName             = "X-Sesame-Context-Binding"
	DefaultContextBinding  = "terminal:default"
	currentHeadMetadataKey = "current_context_head_id:"
)

type contextKey struct{}

func Normalize(binding string) string {
	if trimmed := strings.TrimSpace(binding); trimmed != "" {
		return trimmed
	}
	return DefaultContextBinding
}

func WithContextBinding(ctx context.Context, binding string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, Normalize(binding))
}

func FromContext(ctx context.Context) string {
	if ctx != nil {
		if binding, ok := ctx.Value(contextKey{}).(string); ok {
			return Normalize(binding)
		}
	}
	return DefaultContextBinding
}

func CurrentHeadMetadataKey(binding string) string {
	return currentHeadMetadataKey + Normalize(binding)
}
