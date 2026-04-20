package workspace

import (
	"context"
	"strings"
)

type contextKey struct{}

func WithWorkspaceRoot(ctx context.Context, workspaceRoot string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, strings.TrimSpace(workspaceRoot))
}

func WorkspaceRootFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	workspaceRoot, ok := ctx.Value(contextKey{}).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(workspaceRoot)
}
