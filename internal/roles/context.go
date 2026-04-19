package roles

import (
	"context"
	"strings"
)

type specialistRoleIDContextKey struct{}

func WithSpecialistRoleID(ctx context.Context, roleID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, specialistRoleIDContextKey{}, strings.TrimSpace(roleID))
}

func SpecialistRoleIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	roleID, ok := ctx.Value(specialistRoleIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(roleID)
}
