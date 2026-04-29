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
			permission_profile text not null default '',
			state text not null,
			active_turn_id text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists turns (
			id text primary key,
			session_id text not null,
			turn_kind text not null default '',
			client_turn_id text not null default '',
			state text not null,
			user_message text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists context_heads (
			id text primary key,
			session_id text not null,
			parent_head_id text not null default '',
			source_kind text not null,
			title text not null default '',
			preview text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists runs (
			id text primary key,
			session_id text not null,
			turn_id text not null default '',
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists plans (
			id text primary key,
			run_id text not null,
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists task_records (
			id text primary key,
			run_id text not null,
			plan_id text not null default '',
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists tool_runs (
			id text primary key,
			run_id text not null,
			task_id text not null default '',
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists worktrees (
			id text primary key,
			run_id text not null,
			task_id text not null default '',
			state text not null,
			payload text not null,
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
			kind text not null default '',
			source_session_id text not null default '',
			source_context_head_id text not null default '',
			owner_role_id text not null default '',
			visibility text not null default 'shared',
			status text not null default 'active',
			content text not null,
			source_refs text not null,
			confidence real not null,
			last_used_at text not null default '',
			usage_count integer not null default 0,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists cold_index (
			id text primary key,
			workspace_id text not null,
			owner_role_id text not null default '',
			visibility text not null default 'shared',
			source_type text not null,
			source_id text not null,
			search_text text not null default '',
			summary_line text not null default '',
			files_changed text not null default '[]',
			tools_used text not null default '[]',
			error_types text not null default '[]',
			occurred_at text not null,
			created_at text not null,
			context_ref text not null default '{}'
		);`,
		`create index if not exists idx_cold_index_workspace_role
			on cold_index(workspace_id, owner_role_id);`,
		`create index if not exists idx_cold_index_occurred
			on cold_index(workspace_id, occurred_at);`,
		`create virtual table if not exists cold_index_fts using fts5(
			search_text,
			content='cold_index',
			content_rowid='rowid'
		);`,
		`create table if not exists context_head_summaries (
			session_id text not null,
			context_head_id text not null,
			workspace_root text not null default '',
			source_turn_id text not null default '',
			up_to_item_id integer not null default 0,
			item_count integer not null default 0,
			summary_payload text not null default '',
			created_at text not null,
			updated_at text not null,
			primary key (session_id, context_head_id)
		);`,
		`create table if not exists conversation_items (
			id integer primary key autoincrement,
			session_id text not null,
			context_head_id text not null default '',
			turn_id text not null default '',
			position integer not null,
			kind text not null,
			payload text not null,
			created_at text not null
		);`,
		`create table if not exists turn_usage (
			turn_id text primary key,
			session_id text not null,
			provider text not null default '',
			model text not null default '',
			input_tokens integer not null default 0,
			output_tokens integer not null default 0,
			cached_tokens integer not null default 0,
			cache_hit_rate real not null default 0,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists turn_costs (
			id text primary key,
			turn_id text not null,
			session_id text not null,
			owner_role_id text not null default '',
			input_tokens integer not null default 0,
			output_tokens integer not null default 0,
			cost_usd real not null default 0,
			created_at text not null
		);`,
		`create index if not exists turn_costs_session_created_idx
			on turn_costs(session_id, created_at desc, id asc);`,
		`create unique index if not exists conversation_items_session_position_idx
			on conversation_items(session_id, position);`,
		`create table if not exists conversation_compactions (
			id text primary key,
			session_id text not null,
			context_head_id text not null default '',
			kind text not null,
			generation integer not null,
			start_item_id integer not null default 0,
			end_item_id integer not null default 0,
			start_position integer not null,
			end_position integer not null,
			summary_payload text not null,
			metadata_json text not null default '',
			reason text not null,
			provider_profile text not null,
			created_at text not null
		);`,
		`create table if not exists compaction_qa (
			id text primary key,
			compaction_id text not null,
			session_id text not null,
			compaction_kind text not null,
			source_item_count integer not null default 0,
			summary_text text not null default '',
			source_items_preview text not null default '',
			retained_constraints text not null default '[]',
			lost_constraints text not null default '[]',
			hallucination_check text not null default '',
			confidence real not null default 0,
			review_model text not null default '',
			qa_status text not null default 'pending',
			created_at text not null
		);`,
		`create index if not exists idx_compaction_qa_compaction
			on compaction_qa(compaction_id);`,
		`create index if not exists idx_compaction_qa_session
			on compaction_qa(session_id);`,
		`create table if not exists turn_checkpoints (
			id text primary key,
			turn_id text not null,
			session_id text not null,
			sequence integer not null default 0,
			state text not null default '',
			tool_call_ids text not null default '[]',
			tool_call_names text not null default '[]',
			next_position integer not null default 0,
			completed_tool_ids text not null default '[]',
			tool_results_json text not null default '',
			assistant_items_json text not null default '',
			created_at text not null
		);`,
		`create index if not exists idx_turn_checkpoints_turn
			on turn_checkpoints(turn_id, sequence);`,
		`create table if not exists file_checkpoints (
			id text primary key,
			session_id text not null,
			turn_id text not null,
			tool_call_id text not null default '',
			tool_name text not null default '',
			reason text not null default '',
			git_commit_hash text not null default '',
			files_changed text not null default '[]',
			diff_summary text not null default '',
			parent_checkpoint_id text not null default '',
			created_at text not null
		);`,
		`create index if not exists idx_file_checkpoints_session
			on file_checkpoints(session_id);`,
		`create index if not exists idx_file_checkpoints_turn
			on file_checkpoints(turn_id);`,
		`create table if not exists session_pending_confirmations (
			session_id text primary key,
			source_turn_id text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
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
		`create table if not exists workspace_session_bindings (
			workspace_root text not null,
			binding_kind text not null,
			role text not null default '',
			specialist_role_id text not null default '',
			session_id text not null,
			created_at text not null,
			updated_at text not null,
			primary key (workspace_root, binding_kind, role, specialist_role_id)
		);`,
		`create unique index if not exists workspace_session_bindings_session_idx
			on workspace_session_bindings(workspace_root, session_id);`,
		`create table if not exists workspace_tasks (
			workspace_root text not null,
			task_id text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null,
			primary key (workspace_root, task_id)
		);`,
		`create index if not exists workspace_tasks_root_updated_idx
			on workspace_tasks(workspace_root, updated_at desc, task_id asc);`,
		`create table if not exists workspace_todos (
			workspace_root text primary key,
			payload text not null,
			updated_at text not null
		);`,
		`create table if not exists discord_ingress (
			discord_message_id text primary key,
			guild_id text not null,
			channel_id text not null,
			author_id text not null,
			workspace_root text not null,
			status text not null,
			sesame_turn_id text not null default '',
			error_message text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists discord_ingress_workspace_status_idx
			on discord_ingress(workspace_root, status, updated_at);`,
		`create table if not exists child_agent_specs (
			id text primary key,
			session_id text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists output_contracts (
			id text primary key,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists scheduled_jobs (
			id text primary key,
			workspace_root text not null,
			owner_session_id text not null,
			kind text not null,
			enabled integer not null default 1,
			next_run_at text not null default '',
			last_status text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists scheduled_jobs_due_idx
			on scheduled_jobs(enabled, next_run_at, id asc);`,
		`create table if not exists automations (
			id text primary key,
			workspace_root text not null,
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automations_workspace_state_idx
			on automations(workspace_root, state, updated_at desc, id asc);`,
		`create table if not exists automation_trigger_events (
			event_id text primary key,
			workspace_root text not null,
			automation_id text not null,
			dedupe_key text not null default '',
			signal_kind text not null default '',
			source text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_trigger_events_automation_dedupe_idx
			on automation_trigger_events(automation_id, dedupe_key, observed_at desc, event_id asc);`,
		`create table if not exists automation_simple_runs (
			automation_id text not null,
			dedupe_key text not null,
			owner text not null,
			task_id text not null default '',
			last_status text not null default '',
			last_summary text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null,
			primary key (automation_id, dedupe_key)
		);`,
		`create index if not exists automation_simple_runs_task_idx
			on automation_simple_runs(task_id, updated_at desc, automation_id asc, dedupe_key asc);`,
		`create table if not exists automation_heartbeats (
			automation_id text not null,
			watcher_id text not null,
			workspace_root text not null,
			status text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null,
			primary key (automation_id, watcher_id)
		);`,
		`create index if not exists automation_heartbeats_automation_observed_idx
			on automation_heartbeats(automation_id, observed_at desc, watcher_id asc);`,
		`create table if not exists automation_watchers (
			id text primary key,
			automation_id text not null unique,
			workspace_root text not null,
			state text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_watchers_workspace_state_idx
			on automation_watchers(workspace_root, state, updated_at desc, automation_id asc);`,
		`create table if not exists automation_watcher_holds (
			hold_id text primary key,
			automation_id text not null,
			watcher_id text not null,
			kind text not null,
			owner_id text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_watcher_holds_automation_kind_idx
			on automation_watcher_holds(automation_id, kind, updated_at desc, hold_id asc);`,
		`create table if not exists report_groups (
			id text primary key,
			session_id text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists child_agent_results (
			id text primary key,
			session_id text not null default '',
			agent_id text not null,
			status text not null default '',
			severity text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists child_agent_results_agent_observed_idx
			on child_agent_results(agent_id, observed_at desc, id asc);`,
		`create table if not exists reports (
			id text primary key,
			workspace_root text not null default '',
			session_id text not null,
			source_kind text not null,
			source_id text not null default '',
			severity text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists reports_session_observed_idx
			on reports(session_id, observed_at desc, id asc);`,
		`create table if not exists report_deliveries (
			id text primary key,
			workspace_root text not null default '',
			session_id text not null,
			report_id text not null,
			channel text not null,
			state text not null default '',
			observed_at text not null default '',
			injected_turn_id text not null default '',
			injected_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists report_deliveries_session_channel_state_idx
			on report_deliveries(session_id, channel, state, observed_at desc, id asc);`,
		`create table if not exists digest_records (
			id text primary key,
			session_id text not null default '',
			group_id text not null,
			status text not null default '',
			severity text not null default '',
			window_start text not null default '',
			window_end text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists digest_records_group_window_idx
			on digest_records(group_id, window_end desc, id asc);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	if err := s.migrateConversationArchiveEntries(ctx); err != nil {
		return err
	}

	if err := s.ensureColumn(ctx, "sessions", "system_prompt", `alter table sessions add column system_prompt text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "sessions", "permission_profile", `alter table sessions add column permission_profile text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "turns", "context_head_id", `alter table turns add column context_head_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "turns", "turn_kind", `alter table turns add column turn_kind text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "turns", "execution_mode", `alter table turns add column execution_mode text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "turns", "foreground_lease_id", `alter table turns add column foreground_lease_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "turns", "foreground_lease_expires_at", `alter table turns add column foreground_lease_expires_at text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "kind", `alter table memory_entries add column kind text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "source_session_id", `alter table memory_entries add column source_session_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "source_context_head_id", `alter table memory_entries add column source_context_head_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "owner_role_id", `alter table memory_entries add column owner_role_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "visibility", `alter table memory_entries add column visibility text not null default 'shared'`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "status", `alter table memory_entries add column status text not null default 'active'`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "last_used_at", `alter table memory_entries add column last_used_at text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "memory_entries", "usage_count", `alter table memory_entries add column usage_count integer not null default 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "child_agent_specs", "session_id", `alter table child_agent_specs add column session_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "report_groups", "session_id", `alter table report_groups add column session_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "child_agent_results", "session_id", `alter table child_agent_results add column session_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "digest_records", "session_id", `alter table digest_records add column session_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "reports", "workspace_root", `alter table reports add column workspace_root text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "report_deliveries", "workspace_root", `alter table report_deliveries add column workspace_root text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "conversation_items", "context_head_id", `alter table conversation_items add column context_head_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "conversation_compactions", "context_head_id", `alter table conversation_compactions add column context_head_id text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "conversation_compactions", "start_item_id", `alter table conversation_compactions add column start_item_id integer not null default 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "conversation_compactions", "end_item_id", `alter table conversation_compactions add column end_item_id integer not null default 0`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "conversation_compactions", "metadata_json", `alter table conversation_compactions add column metadata_json text not null default ''`); err != nil {
		return err
	}
	indexStmts := []string{
		`create index if not exists child_agent_specs_session_idx
			on child_agent_specs(session_id, updated_at desc, id asc);`,
		`create index if not exists report_groups_session_idx
			on report_groups(session_id, updated_at desc, id asc);`,
		`create index if not exists child_agent_results_session_observed_idx
			on child_agent_results(session_id, observed_at desc, id asc);`,
		`create index if not exists digest_records_session_group_window_idx
			on digest_records(session_id, group_id, window_end desc, id asc);`,
		`create index if not exists reports_workspace_observed_idx
			on reports(workspace_root, observed_at desc, id asc);`,
		`create index if not exists report_deliveries_workspace_channel_state_idx
			on report_deliveries(workspace_root, channel, state, observed_at desc, id asc);`,
		`create index if not exists conversation_items_session_head_position_idx
			on conversation_items(session_id, context_head_id, position, id);`,
		`create index if not exists conversation_items_session_turn_id_idx
			on conversation_items(session_id, turn_id, id);`,
		`create index if not exists conversation_compactions_session_head_generation_idx
			on conversation_compactions(session_id, context_head_id, generation, created_at, id);`,
		`create index if not exists memory_entries_scope_kind_workspace_idx
			on memory_entries(scope, kind, workspace_id, updated_at desc, created_at desc);`,
	}
	for _, stmt := range indexStmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `delete from memory_entries where scope = 'session'`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`drop table if exists permission_requests;`,
		`drop table if exists turn_continuations;`,
		`drop table if exists pending_task_completions;`,
		`drop table if exists head_memories;`,
		`drop table if exists head_memory;`,
		`drop table if exists memory_candidates;`,
		`drop table if exists conversation_summaries;`,
		`drop table if exists session_memories;`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migrateConversationArchiveEntries(ctx context.Context) error {
	stmts := []string{
		`create table if not exists conversation_archive_entries (
			id text primary key,
			session_id text not null,
			range_label text not null default '',
			turn_start integer not null default 0,
			turn_end integer not null default 0,
			item_count integer not null default 0,
			summary text not null default '',
			decisions text not null default '[]',
			files_changed text not null default '[]',
			errors_and_fixes text not null default '[]',
			tools_used text not null default '[]',
			keywords text not null default '[]',
			is_computed integer not null default 0,
			created_at text not null
		);`,
		`create index if not exists conversation_archive_entries_session_created_idx
			on conversation_archive_entries(session_id, created_at desc, id asc);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
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
