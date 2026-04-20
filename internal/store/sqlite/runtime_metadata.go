package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type queryRowContexter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const canonicalSessionMetadataKey = "canonical_session_id"

func (s *Store) GetCanonicalSessionID(ctx context.Context) (string, bool, error) {
	return getRuntimeMetadataValue(ctx, s.db, canonicalSessionMetadataKey)
}

func (s *Store) SetCanonicalSessionID(ctx context.Context, sessionID string) error {
	return setRuntimeMetadataValue(ctx, s.db, canonicalSessionMetadataKey, sessionID)
}

func (s *Store) GetCurrentContextHeadID(ctx context.Context) (string, bool, error) {
	binding := sessionbinding.FromContext(ctx)
	role := sessionrole.FromContext(ctx)
	specialistRoleID := rolectx.SpecialistRoleIDFromContext(ctx)
	workspaceRoot := workspace.WorkspaceRootFromContext(ctx)
	key := currentHeadMetadataKey(binding, workspaceRoot, role, specialistRoleID)
	currentHeadID, found, err := getRuntimeMetadataValue(ctx, s.db, key)
	if err != nil || found {
		return currentHeadID, found, err
	}
	key = legacyCurrentHeadMetadataKey(binding, role, specialistRoleID)
	return getRuntimeMetadataValue(ctx, s.db, key)
}

func (s *Store) SetCurrentContextHeadID(ctx context.Context, headID string) error {
	binding := sessionbinding.FromContext(ctx)
	role := sessionrole.FromContext(ctx)
	specialistRoleID := rolectx.SpecialistRoleIDFromContext(ctx)
	workspaceRoot := workspace.WorkspaceRootFromContext(ctx)
	key := currentHeadMetadataKey(binding, workspaceRoot, role, specialistRoleID)
	return setRuntimeMetadataValue(ctx, s.db, key, strings.TrimSpace(headID))
}

func (s *Store) GetRoleSessionID(ctx context.Context, workspaceRoot string, role types.SessionRole) (string, bool, error) {
	role = sessionrole.Normalize(string(role))
	return getRuntimeMetadataValue(ctx, s.db, roleSessionMetadataKey(workspaceRoot, role))
}

func (s *Store) SetRoleSessionID(ctx context.Context, workspaceRoot string, role types.SessionRole, sessionID string) error {
	role = sessionrole.Normalize(string(role))
	return setRuntimeMetadataValue(ctx, s.db, roleSessionMetadataKey(workspaceRoot, role), strings.TrimSpace(sessionID))
}

func (s *Store) GetSpecialistSessionID(ctx context.Context, workspaceRoot, roleID string) (string, bool, error) {
	roleID = normalizeSpecialistRoleID(roleID)
	if roleID == "" {
		return "", false, nil
	}
	return getRuntimeMetadataValue(ctx, s.db, specialistSessionMetadataKey(workspaceRoot, roleID))
}

func (s *Store) SetSpecialistSessionID(ctx context.Context, workspaceRoot, roleID, sessionID string) error {
	roleID = normalizeSpecialistRoleID(roleID)
	if roleID == "" {
		return errors.New("specialist role id is required")
	}
	return setRuntimeMetadataValue(ctx, s.db, specialistSessionMetadataKey(workspaceRoot, roleID), strings.TrimSpace(sessionID))
}

func currentHeadMetadataKey(binding string, workspaceRoot string, role types.SessionRole, specialistRoleID string) string {
	normalizedWorkspaceRoot := strings.TrimSpace(workspaceRoot)
	encodedWorkspaceRoot := base64.RawURLEncoding.EncodeToString([]byte(normalizedWorkspaceRoot))
	if specialistRoleID = normalizeSpecialistRoleID(specialistRoleID); specialistRoleID != "" {
		encodedRoleID := base64.RawURLEncoding.EncodeToString([]byte(specialistRoleID))
		return sessionbinding.CurrentHeadMetadataKey(binding) + ":" + encodedWorkspaceRoot + ":specialist:" + encodedRoleID
	}
	return sessionbinding.CurrentHeadMetadataKey(binding) + ":" + encodedWorkspaceRoot + ":role:" + string(sessionrole.Normalize(string(role)))
}

func legacyCurrentHeadMetadataKey(binding string, role types.SessionRole, specialistRoleID string) string {
	if specialistRoleID = normalizeSpecialistRoleID(specialistRoleID); specialistRoleID != "" {
		encodedRoleID := base64.RawURLEncoding.EncodeToString([]byte(specialistRoleID))
		return sessionbinding.CurrentHeadMetadataKey(binding) + ":specialist:" + encodedRoleID
	}
	return sessionbinding.CurrentHeadMetadataKey(binding) + ":role:" + string(sessionrole.Normalize(string(role)))
}

func roleSessionMetadataKey(workspaceRoot string, role types.SessionRole) string {
	normalized := strings.TrimSpace(workspaceRoot)
	encodedRoot := base64.RawURLEncoding.EncodeToString([]byte(normalized))
	return "role_session:" + encodedRoot + ":" + string(sessionrole.Normalize(string(role)))
}

func specialistSessionMetadataKey(workspaceRoot, roleID string) string {
	normalizedWorkspaceRoot := strings.TrimSpace(workspaceRoot)
	encodedRoot := base64.RawURLEncoding.EncodeToString([]byte(normalizedWorkspaceRoot))
	encodedRoleID := base64.RawURLEncoding.EncodeToString([]byte(normalizeSpecialistRoleID(roleID)))
	return "specialist_session:" + encodedRoot + ":" + encodedRoleID
}

func normalizeSpecialistRoleID(roleID string) string {
	roleID = strings.TrimSpace(roleID)
	switch roleID {
	case "", string(types.SessionRoleMainParent):
		return ""
	default:
		return roleID
	}
}

func getRuntimeMetadataValue(ctx context.Context, queryer queryRowContexter, key string) (string, bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx, `
		select value
		from runtime_metadata
		where key = ?
	`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func setRuntimeMetadataValue(ctx context.Context, execer execContexter, key, value string) error {
	_, err := execer.ExecContext(ctx, `
		insert into runtime_metadata (key, value, updated_at)
		values (?, ?, ?)
		on conflict(key) do update set
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, time.Now().UTC().Format(timeLayout))
	return err
}
