package sqlite

import "context"

func (s *Store) backfillLegacyChatMemoryKeys(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		update conversation_items
		set context_head_id = coalesce((
			select turns.context_head_id
			from turns
			where turns.id = conversation_items.turn_id
			  and turns.session_id = conversation_items.session_id
		), context_head_id)
		where context_head_id = '' and turn_id <> ''
	`); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `
		update conversation_compactions
		set start_item_id = coalesce((
			select conversation_items.id
			from conversation_items
			where conversation_items.session_id = conversation_compactions.session_id
			  and conversation_items.position = conversation_compactions.start_position
		), start_item_id)
		where start_item_id = 0
	`); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `
		update conversation_compactions
		set end_item_id = coalesce((
			select conversation_items.id
			from conversation_items
			where conversation_items.session_id = conversation_compactions.session_id
			  and conversation_items.position = conversation_compactions.end_position
		), end_item_id)
		where end_item_id = 0
	`); err != nil {
		return err
	}

	return nil
}
