package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/types"
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
			client_turn_id text not null default '',
			state text not null,
			user_message text not null,
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
		`create table if not exists permission_requests (
			id text primary key,
			session_id text not null,
			turn_id text not null default '',
			run_id text not null default '',
			task_id text not null default '',
			tool_run_id text not null default '',
			status text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table if not exists turn_continuations (
			id text primary key,
			session_id text not null,
			turn_id text not null,
			permission_request_id text not null default '',
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
			metadata_json text not null default '',
			reason text not null,
			provider_profile text not null,
			created_at text not null
		);`,
		`create table if not exists session_memories (
			session_id text primary key,
			workspace_root text not null default '',
			source_turn_id text not null default '',
			up_to_position integer not null default 0,
			item_count integer not null default 0,
			summary_payload text not null default '',
			created_at text not null,
			updated_at text not null
		);`,
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
		`create table if not exists automation_incidents (
			id text primary key,
			automation_id text not null,
			workspace_root text not null,
			status text not null,
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_incidents_automation_status_idx
			on automation_incidents(automation_id, status, observed_at desc, id asc);`,
		`create table if not exists automation_trigger_events (
			event_id text primary key,
			workspace_root text not null,
			automation_id text not null,
			incident_id text not null default '',
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
		`create table if not exists automation_incident_phase_states (
			incident_id text not null,
			phase text not null,
			automation_id text not null,
			workspace_root text not null,
			status text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null,
			primary key (incident_id, phase)
		);`,
		`create index if not exists automation_incident_phase_states_incident_idx
			on automation_incident_phase_states(incident_id, updated_at desc, phase asc);`,
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
		`create table if not exists automation_dispatch_attempts (
			dispatch_id text primary key,
			workspace_root text not null,
			automation_id text not null,
			incident_id text not null,
			phase text not null,
			status text not null,
			task_id text not null default '',
			background_session_id text not null default '',
			background_turn_id text not null default '',
			permission_request_id text not null default '',
			continuation_id text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_dispatch_attempts_incident_status_idx
			on automation_dispatch_attempts(incident_id, status, updated_at desc, dispatch_id asc);`,
		`create table if not exists automation_delivery_records (
			delivery_id text primary key,
			workspace_root text not null,
			automation_id text not null,
			incident_id text not null,
			dispatch_id text not null,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists automation_delivery_records_dispatch_idx
			on automation_delivery_records(dispatch_id, updated_at desc, delivery_id asc);`,
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
		`create table if not exists pending_task_completions (
			id text primary key,
			session_id text not null,
			task_id text not null,
			observed_at text not null default '',
			injected_turn_id text not null default '',
			injected_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists pending_task_completions_session_injected_idx
			on pending_task_completions(session_id, injected_turn_id, observed_at desc, id asc);`,
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
		`create table if not exists report_mailbox_items (
			id text primary key,
			workspace_root text not null default '',
			session_id text not null,
			source_kind text not null,
			source_id text not null default '',
			severity text not null default '',
			observed_at text not null default '',
			injected_turn_id text not null default '',
			injected_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create index if not exists report_mailbox_items_session_injected_idx
			on report_mailbox_items(session_id, injected_turn_id, observed_at desc, id asc);`,
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

	if err := s.ensureColumn(ctx, "sessions", "system_prompt", `alter table sessions add column system_prompt text not null default ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "sessions", "permission_profile", `alter table sessions add column permission_profile text not null default ''`); err != nil {
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
	if err := s.ensureColumn(ctx, "report_mailbox_items", "workspace_root", `alter table report_mailbox_items add column workspace_root text not null default ''`); err != nil {
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
	}
	for _, stmt := range indexStmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.backfillLegacyReportingSessions(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyReportMailboxItems(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyReportWorkspaceRoots(ctx); err != nil {
		return err
	}
	if err := s.ensureAutomationHeartbeatSchema(ctx); err != nil {
		return err
	}
	if err := s.ensureAutomationDispatchDeliverySchema(ctx); err != nil {
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

func (s *Store) ensureAutomationHeartbeatSchema(ctx context.Context) error {
	hasWatcherID, err := s.tableHasColumn(ctx, "automation_heartbeats", "watcher_id")
	if err != nil {
		return err
	}
	hasLegacyID, err := s.tableHasColumn(ctx, "automation_heartbeats", "id")
	if err != nil {
		return err
	}
	if hasWatcherID && !hasLegacyID {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `drop index if exists automation_heartbeats_automation_observed_idx`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `alter table automation_heartbeats rename to automation_heartbeats_legacy`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		create table automation_heartbeats (
			automation_id text not null,
			watcher_id text not null,
			workspace_root text not null,
			status text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null,
			primary key (automation_id, watcher_id)
		)
	`); err != nil {
		return err
	}

	insertStmt := `
		insert into automation_heartbeats (
			automation_id, watcher_id, workspace_root, status, observed_at, payload, created_at, updated_at
		)
		select
			automation_id,
			case
				when trim(id) <> '' then 'legacy:' || trim(id)
				else 'legacy:' || automation_id
			end,
			workspace_root,
			status,
			observed_at,
			payload,
			created_at,
			updated_at
		from automation_heartbeats_legacy
	`
	if hasWatcherID {
		insertStmt = `
			insert into automation_heartbeats (
				automation_id, watcher_id, workspace_root, status, observed_at, payload, created_at, updated_at
			)
			select
				automation_id,
				case
					when trim(watcher_id) <> '' then trim(watcher_id)
					when trim(id) <> '' then 'legacy:' || trim(id)
					else 'legacy:' || automation_id
				end,
				workspace_root,
				status,
				observed_at,
				payload,
				created_at,
				updated_at
			from automation_heartbeats_legacy
		`
	}
	if _, err := tx.ExecContext(ctx, insertStmt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `drop table automation_heartbeats_legacy`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		create index automation_heartbeats_automation_observed_idx
			on automation_heartbeats(automation_id, observed_at desc, watcher_id asc)
	`); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ensureAutomationDispatchDeliverySchema(ctx context.Context) error {
	hasLegacyDispatch, err := s.tableExists(ctx, "dispatch_attempts")
	if err != nil {
		return err
	}
	hasLegacyDelivery, err := s.tableExists(ctx, "delivery_records")
	if err != nil {
		return err
	}
	if !hasLegacyDispatch && !hasLegacyDelivery {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if hasLegacyDispatch {
		rows, err := tx.QueryContext(ctx, `
			select
				dispatch_id,
				workspace_root,
				automation_id,
				incident_id,
				phase,
				status,
				task_id,
				background_session_id,
				background_turn_id,
				permission_request_id,
				continuation_id,
				payload,
				created_at,
				updated_at
			from dispatch_attempts
			order by updated_at asc, created_at asc, dispatch_id asc, attempt asc
		`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				dispatchID          string
				workspaceRoot       string
				automationID        string
				incidentID          string
				phase               string
				status              string
				taskID              string
				backgroundSessionID string
				backgroundTurnID    string
				permissionRequestID string
				continuationID      string
				payload             string
				createdAt           string
				updatedAt           string
			)
			if err := rows.Scan(
				&dispatchID,
				&workspaceRoot,
				&automationID,
				&incidentID,
				&phase,
				&status,
				&taskID,
				&backgroundSessionID,
				&backgroundTurnID,
				&permissionRequestID,
				&continuationID,
				&payload,
				&createdAt,
				&updatedAt,
			); err != nil {
				rows.Close()
				return err
			}
			if strings.TrimSpace(dispatchID) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				insert into automation_dispatch_attempts (
					dispatch_id, workspace_root, automation_id, incident_id, phase, status,
					task_id, background_session_id, background_turn_id, permission_request_id,
					continuation_id, payload, created_at, updated_at
				)
				values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				on conflict(dispatch_id) do update set
					workspace_root = excluded.workspace_root,
					automation_id = excluded.automation_id,
					incident_id = excluded.incident_id,
					phase = excluded.phase,
					status = excluded.status,
					task_id = excluded.task_id,
					background_session_id = excluded.background_session_id,
					background_turn_id = excluded.background_turn_id,
					permission_request_id = excluded.permission_request_id,
					continuation_id = excluded.continuation_id,
					payload = excluded.payload,
					updated_at = excluded.updated_at
			`,
				strings.TrimSpace(dispatchID),
				strings.TrimSpace(workspaceRoot),
				strings.TrimSpace(automationID),
				strings.TrimSpace(incidentID),
				strings.TrimSpace(phase),
				strings.TrimSpace(status),
				strings.TrimSpace(taskID),
				strings.TrimSpace(backgroundSessionID),
				strings.TrimSpace(backgroundTurnID),
				strings.TrimSpace(permissionRequestID),
				strings.TrimSpace(continuationID),
				payload,
				createdAt,
				updatedAt,
			); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `drop index if exists dispatch_attempts_incident_status_idx`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `drop table dispatch_attempts`); err != nil {
			return err
		}
	}

	if hasLegacyDelivery {
		rows, err := tx.QueryContext(ctx, `
			select
				delivery_id,
				workspace_root,
				automation_id,
				incident_id,
				dispatch_id,
				payload,
				created_at,
				updated_at
			from delivery_records
			order by updated_at asc, created_at asc, delivery_id asc
		`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				deliveryID    string
				workspaceRoot string
				automationID  string
				incidentID    string
				dispatchID    string
				payload       string
				createdAt     string
				updatedAt     string
			)
			if err := rows.Scan(
				&deliveryID,
				&workspaceRoot,
				&automationID,
				&incidentID,
				&dispatchID,
				&payload,
				&createdAt,
				&updatedAt,
			); err != nil {
				rows.Close()
				return err
			}
			if strings.TrimSpace(deliveryID) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				insert into automation_delivery_records (
					delivery_id, workspace_root, automation_id, incident_id, dispatch_id, payload, created_at, updated_at
				)
				values (?, ?, ?, ?, ?, ?, ?, ?)
				on conflict(delivery_id) do update set
					workspace_root = excluded.workspace_root,
					automation_id = excluded.automation_id,
					incident_id = excluded.incident_id,
					dispatch_id = excluded.dispatch_id,
					payload = excluded.payload,
					updated_at = excluded.updated_at
			`,
				strings.TrimSpace(deliveryID),
				strings.TrimSpace(workspaceRoot),
				strings.TrimSpace(automationID),
				strings.TrimSpace(incidentID),
				strings.TrimSpace(dispatchID),
				payload,
				createdAt,
				updatedAt,
			); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `drop index if exists delivery_records_dispatch_idx`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `drop table delivery_records`); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) tableExists(ctx context.Context, table string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(*)
		from sqlite_master
		where type = 'table' and name = ?
	`, strings.TrimSpace(table)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) tableHasColumn(ctx context.Context, table, column string) (bool, error) {
	var count int
	query := fmt.Sprintf("select count(*) from pragma_table_info('%s') where name = ?", table)
	if err := s.db.QueryRowContext(ctx, query, column).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) backfillLegacyReportingSessions(ctx context.Context) error {
	if err := s.backfillLegacyChildAgentSpecs(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyReportGroups(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyChildAgentResults(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyDigestRecords(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) backfillLegacyChildAgentSpecs(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select id, payload, created_at, updated_at
		from child_agent_specs
		where session_id = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return err
		}
		var spec types.ChildAgentSpec
		if err := json.Unmarshal([]byte(payload), &spec); err != nil {
			return err
		}
		spec.AgentID = firstNonEmptyReportingString(spec.AgentID, id)
		applyLegacyReportingTimestamps(&spec, createdAt, updatedAt)
		if strings.TrimSpace(spec.SessionID) == "" {
			spec.SessionID = s.scheduledJobOwnerSession(ctx, spec.AgentID)
		}
		if strings.TrimSpace(spec.SessionID) == "" {
			continue
		}
		if err := s.UpsertChildAgentSpec(ctx, spec); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillLegacyReportGroups(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select id, payload, created_at, updated_at
		from report_groups
		where session_id = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return err
		}
		var group types.ReportGroup
		if err := json.Unmarshal([]byte(payload), &group); err != nil {
			return err
		}
		group.GroupID = firstNonEmptyReportingString(group.GroupID, id)
		applyLegacyReportingTimestamps(&group, createdAt, updatedAt)
		if strings.TrimSpace(group.SessionID) == "" {
			for _, source := range group.Sources {
				group.SessionID = s.scheduledJobOwnerSession(ctx, source)
				if strings.TrimSpace(group.SessionID) != "" {
					break
				}
			}
		}
		if strings.TrimSpace(group.SessionID) == "" {
			continue
		}
		if err := s.UpsertReportGroup(ctx, group); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillLegacyChildAgentResults(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select id, payload, created_at, updated_at
		from child_agent_results
		where session_id = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return err
		}
		var result types.ChildAgentResult
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			return err
		}
		result.ResultID = firstNonEmptyReportingString(result.ResultID, id)
		applyLegacyReportingTimestamps(&result, createdAt, updatedAt)
		if strings.TrimSpace(result.SessionID) == "" {
			result.SessionID = s.scheduledJobOwnerSession(ctx, result.AgentID)
		}
		if strings.TrimSpace(result.SessionID) == "" {
			for _, groupID := range result.ReportGroupRefs {
				group, ok, err := s.GetReportGroup(ctx, groupID)
				if err != nil {
					return err
				}
				if ok && strings.TrimSpace(group.SessionID) != "" {
					result.SessionID = strings.TrimSpace(group.SessionID)
					break
				}
			}
		}
		if strings.TrimSpace(result.SessionID) == "" {
			continue
		}
		if err := s.UpsertChildAgentResult(ctx, result); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillLegacyDigestRecords(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select id, payload, created_at, updated_at
		from digest_records
		where session_id = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return err
		}
		var digest types.DigestRecord
		if err := json.Unmarshal([]byte(payload), &digest); err != nil {
			return err
		}
		digest.DigestID = firstNonEmptyReportingString(digest.DigestID, id)
		applyLegacyReportingTimestamps(&digest, createdAt, updatedAt)
		if strings.TrimSpace(digest.SessionID) == "" && strings.TrimSpace(digest.GroupID) != "" {
			group, ok, err := s.GetReportGroup(ctx, digest.GroupID)
			if err != nil {
				return err
			}
			if ok {
				digest.SessionID = strings.TrimSpace(group.SessionID)
			}
		}
		if strings.TrimSpace(digest.SessionID) == "" {
			continue
		}
		if err := s.UpsertDigestRecord(ctx, digest); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillLegacyReportMailboxItems(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_mailbox_items
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			payload        string
			observedAt     string
			injectedTurnID string
			injectedAt     string
			createdAt      string
			updatedAt      string
		)
		if err := rows.Scan(&payload, &observedAt, &injectedTurnID, &injectedAt, &createdAt, &updatedAt); err != nil {
			return err
		}
		var item types.ReportMailboxItem
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			return err
		}
		applyLegacyReportMailboxTimes(&item, observedAt, injectedTurnID, injectedAt, createdAt, updatedAt)
		if strings.TrimSpace(item.WorkspaceRoot) == "" && strings.TrimSpace(item.SessionID) != "" {
			item.WorkspaceRoot = s.workspaceRootForSession(ctx, item.SessionID)
		}
		report, delivery := mailboxItemToRecordDelivery(item)
		if err := s.UpsertReport(ctx, report); err != nil {
			return err
		}
		if err := s.UpsertReportDelivery(ctx, delivery); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillLegacyReportWorkspaceRoots(ctx context.Context) error {
	stmts := []string{
		`
		update reports
		set workspace_root = (
			select workspace_root
			from sessions
			where sessions.id = reports.session_id
		)
		where workspace_root = '' and session_id != ''
			and exists (select 1 from sessions where sessions.id = reports.session_id and sessions.workspace_root != '')
		`,
		`
		update report_deliveries
		set workspace_root = (
			select workspace_root
			from sessions
			where sessions.id = report_deliveries.session_id
		)
		where workspace_root = '' and session_id != ''
			and exists (select 1 from sessions where sessions.id = report_deliveries.session_id and sessions.workspace_root != '')
		`,
		`
		update report_mailbox_items
		set workspace_root = (
			select workspace_root
			from sessions
			where sessions.id = report_mailbox_items.session_id
		)
		where workspace_root = '' and session_id != ''
			and exists (select 1 from sessions where sessions.id = report_mailbox_items.session_id and sessions.workspace_root != '')
		`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) scheduledJobOwnerSession(ctx context.Context, jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ""
	}
	var ownerSessionID string
	err := s.db.QueryRowContext(ctx, `
		select owner_session_id
		from scheduled_jobs
		where id = ?
	`, jobID).Scan(&ownerSessionID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(ownerSessionID)
}

func applyLegacyReportingTimestamps(value any, createdAtRaw, updatedAtRaw string) {
	createdAt, err := parsePendingOptionalTime(createdAtRaw)
	if err != nil {
		return
	}
	updatedAt, err := parsePendingOptionalTime(updatedAtRaw)
	if err != nil {
		updatedAt = createdAt
	}
	applyReportingTimestamps(value, createdAt, updatedAt)
}

func applyLegacyReportMailboxTimes(item *types.ReportMailboxItem, observedAtRaw, injectedTurnID, injectedAtRaw, createdAtRaw, updatedAtRaw string) {
	if item == nil {
		return
	}
	if parsed, err := parsePendingOptionalTime(observedAtRaw); err == nil {
		item.ObservedAt = parsed
	}
	item.InjectedTurnID = strings.TrimSpace(injectedTurnID)
	if parsed, err := parsePendingOptionalTime(injectedAtRaw); err == nil {
		item.InjectedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(createdAtRaw); err == nil {
		item.CreatedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(updatedAtRaw); err == nil {
		item.UpdatedAt = parsed
	}
}

func firstNonEmptyReportingString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
