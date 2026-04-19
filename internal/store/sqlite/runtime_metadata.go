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
	key := currentHeadMetadataKey(binding, role, specialistRoleID)
	value, ok, err := getRuntimeMetadataValue(ctx, s.db, key)
	if err != nil || ok || role != types.SessionRoleMainParent || normalizeSpecialistRoleID(specialistRoleID) != "" {
		return value, ok, err
	}
	return getRuntimeMetadataValue(ctx, s.db, sessionbinding.CurrentHeadMetadataKey(binding))
}

func (s *Store) SetCurrentContextHeadID(ctx context.Context, headID string) error {
	binding := sessionbinding.FromContext(ctx)
	role := sessionrole.FromContext(ctx)
	specialistRoleID := rolectx.SpecialistRoleIDFromContext(ctx)
	if err := setRuntimeMetadataValue(ctx, s.db, currentHeadMetadataKey(binding, role, specialistRoleID), headID); err != nil {
		return err
	}
	if role == types.SessionRoleMainParent && normalizeSpecialistRoleID(specialistRoleID) == "" {
		return setRuntimeMetadataValue(ctx, s.db, sessionbinding.CurrentHeadMetadataKey(binding), headID)
	}
	return nil
}

func (s *Store) GetRoleSessionID(ctx context.Context, workspaceRoot string, role types.SessionRole) (string, bool, error) {
	role = sessionrole.Normalize(string(role))
	if role == types.SessionRoleMonitoringParent {
		if sessionID, ok, err := s.GetSpecialistSessionID(ctx, workspaceRoot, monitoringSpecialistRoleID); err != nil || ok {
			return sessionID, ok, err
		}
	}
	if role == types.SessionRoleMainParent {
		if sessionID, ok, err := getRuntimeMetadataValue(ctx, s.db, roleSessionMetadataKey(workspaceRoot, role)); err != nil {
			return "", false, err
		} else if ok {
			return sessionID, true, nil
		}
	}
	if role == types.SessionRoleMainParent {
		return s.GetCanonicalSessionID(ctx)
	}
	return getRuntimeMetadataValue(ctx, s.db, roleSessionMetadataKey(workspaceRoot, role))
}

func (s *Store) SetRoleSessionID(ctx context.Context, workspaceRoot string, role types.SessionRole, sessionID string) error {
	role = sessionrole.Normalize(string(role))
	if role == types.SessionRoleMonitoringParent {
		return s.SetSpecialistSessionID(ctx, workspaceRoot, monitoringSpecialistRoleID, strings.TrimSpace(sessionID))
	}
	if err := setRuntimeMetadataValue(ctx, s.db, roleSessionMetadataKey(workspaceRoot, role), strings.TrimSpace(sessionID)); err != nil {
		return err
	}
	if role == types.SessionRoleMainParent {
		return s.SetCanonicalSessionID(ctx, strings.TrimSpace(sessionID))
	}
	return nil
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

func currentHeadMetadataKey(binding string, role types.SessionRole, specialistRoleID string) string {
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
	case string(types.SessionRoleMonitoringParent), monitoringSpecialistRoleID:
		return monitoringSpecialistRoleID
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
