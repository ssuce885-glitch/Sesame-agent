package sqlite

import (
	"context"
	"fmt"
)

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`create table if not exists sessions (
			id text primary key,
			workspace_root text not null,
			system_prompt text not null default '',
			state text not null,
			active_turn_id text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists turns (
			id text primary key,
			session_id text not null,
			client_turn_id text not null default '',
			state text not null,
			user_message text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists events (
			seq integer primary key autoincrement,
			id text not null,
			session_id text not null,
			turn_id text not null default '',
			type text not null,
			time text not null,
			payload text not null
		);`,
		`create table if not exists memory_entries (
			id text primary key,
			scope text not null,
			workspace_id text not null default '',
			content text not null,
			source_refs text not null,
			confidence real not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists memory_candidates (
			id text primary key,
			scope text not null,
			workspace_id text not null default '',
			content text not null,
			source_refs text not null,
			confidence real not null,
			created_at text not null,
			approved integer not null default 0
		);`,
		`create table if not exists conversation_items (
			id integer primary key autoincrement,
			session_id text not null,
			turn_id text not null default '',
			position integer not null,
			kind text not null,
			payload text not null,
			created_at text not null
		);`,
		`create unique index if not exists conversation_items_session_position_idx
			on conversation_items(session_id, position);`,
		`create table if not exists conversation_summaries (
			id integer primary key autoincrement,
			session_id text not null,
			up_to_position integer not null,
			payload text not null,
			created_at text not null
		);`,
		`create table if not exists conversation_compactions (
			id text primary key,
			session_id text not null,
			kind text not null,
			generation integer not null,
			start_position integer not null,
			end_position integer not null,
			summary_payload text not null,
			reason text not null,
			provider_profile text not null,
			created_at text not null
		);`,
		`create table if not exists provider_cache_entries (
			id text primary key,
			session_id text not null,
			provider text not null,
			capability_profile text not null,
			cache_kind text not null,
			external_ref text not null,
			parent_external_ref text not null default '',
			generation integer not null,
			status text not null,
			expires_at text not null default '',
			last_used_at text not null default '',
			metadata_json text not null default '{}',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists provider_cache_heads (
			session_id text not null,
			provider text not null,
			capability_profile text not null,
			active_session_ref text not null default '',
			active_prefix_ref text not null default '',
			active_generation integer not null default 0,
			updated_at text not null,
			primary key (session_id, provider, capability_profile)
		);`,
		`create table if not exists runtime_metadata (
			key text primary key,
			value text not null,
			updated_at text not null
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	if err := s.ensureColumn(ctx, "sessions", "system_prompt", `alter table sessions add column system_prompt text not null default ''`); err != nil {
		return err
	}

	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, alterStmt string) error {
	var count int
	query := fmt.Sprintf("select count(*) from pragma_table_info('%s') where name = ?", table)
	if err := s.db.QueryRowContext(ctx, query, column).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, alterStmt)
	return err
}
