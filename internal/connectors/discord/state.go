package discord

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const ingressTimeLayout = time.RFC3339Nano

type IngressRecord struct {
	DiscordMessageID string
	GuildID          string
	ChannelID        string
	AuthorID         string
	WorkspaceRoot    string
	Status           string
	SesameTurnID     string
	ErrorMessage     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type StateStore struct {
	db *sql.DB
}

func NewStateStore(db *sql.DB) *StateStore {
	return &StateStore{db: db}
}

func (s *StateStore) GetDiscordIngress(ctx context.Context, discordMessageID string) (IngressRecord, bool, error) {
	if s == nil || s.db == nil {
		return IngressRecord{}, false, errors.New("discord state store is not configured")
	}

	var (
		rec       IngressRecord
		createdAt string
		updatedAt string
	)

	err := s.db.QueryRowContext(ctx, `
		select discord_message_id, guild_id, channel_id, author_id, workspace_root,
		       status, sesame_turn_id, error_message, created_at, updated_at
		from discord_ingress
		where discord_message_id = ?
	`, strings.TrimSpace(discordMessageID)).Scan(
		&rec.DiscordMessageID,
		&rec.GuildID,
		&rec.ChannelID,
		&rec.AuthorID,
		&rec.WorkspaceRoot,
		&rec.Status,
		&rec.SesameTurnID,
		&rec.ErrorMessage,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return IngressRecord{}, false, nil
	}
	if err != nil {
		return IngressRecord{}, false, err
	}

	rec.CreatedAt, err = time.Parse(ingressTimeLayout, createdAt)
	if err != nil {
		return IngressRecord{}, false, err
	}
	rec.UpdatedAt, err = time.Parse(ingressTimeLayout, updatedAt)
	if err != nil {
		return IngressRecord{}, false, err
	}
	return rec, true, nil
}

func (s *StateStore) UpsertDiscordIngress(ctx context.Context, rec IngressRecord) error {
	if s == nil || s.db == nil {
		return errors.New("discord state store is not configured")
	}

	now := time.Now().UTC()
	createdAt := rec.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := rec.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		insert into discord_ingress (
			discord_message_id, guild_id, channel_id, author_id, workspace_root,
			status, sesame_turn_id, error_message, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(discord_message_id) do update set
			guild_id = excluded.guild_id,
			channel_id = excluded.channel_id,
			author_id = excluded.author_id,
			workspace_root = excluded.workspace_root,
			status = excluded.status,
			sesame_turn_id = excluded.sesame_turn_id,
			error_message = excluded.error_message,
			updated_at = excluded.updated_at
	`,
		strings.TrimSpace(rec.DiscordMessageID),
		strings.TrimSpace(rec.GuildID),
		strings.TrimSpace(rec.ChannelID),
		strings.TrimSpace(rec.AuthorID),
		strings.TrimSpace(rec.WorkspaceRoot),
		strings.TrimSpace(rec.Status),
		strings.TrimSpace(rec.SesameTurnID),
		rec.ErrorMessage,
		createdAt.Format(ingressTimeLayout),
		updatedAt.Format(ingressTimeLayout),
	)
	return err
}

func (s *StateStore) SetDiscordIngressTurnID(ctx context.Context, discordMessageID, turnID string) error {
	if s == nil || s.db == nil {
		return errors.New("discord state store is not configured")
	}
	_, err := s.db.ExecContext(ctx, `
		update discord_ingress
		set sesame_turn_id = ?, updated_at = ?
		where discord_message_id = ?
	`,
		strings.TrimSpace(turnID),
		time.Now().UTC().Format(ingressTimeLayout),
		strings.TrimSpace(discordMessageID),
	)
	return err
}

func (s *StateStore) SetDiscordIngressStatus(ctx context.Context, discordMessageID, status, errorMessage string) error {
	if s == nil || s.db == nil {
		return errors.New("discord state store is not configured")
	}
	_, err := s.db.ExecContext(ctx, `
		update discord_ingress
		set status = ?, error_message = ?, updated_at = ?
		where discord_message_id = ?
	`,
		strings.TrimSpace(status),
		errorMessage,
		time.Now().UTC().Format(ingressTimeLayout),
		strings.TrimSpace(discordMessageID),
	)
	return err
}
