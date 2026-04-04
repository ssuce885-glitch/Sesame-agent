package sqlite

import "context"

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`create table if not exists sessions (
			id text primary key,
			workspace_root text not null,
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
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
