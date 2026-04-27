package discord

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"go-agent/internal/session"
	"go-agent/internal/types"

	_ "modernc.org/sqlite"
)

type bridgeTestStore struct{}

func (bridgeTestStore) EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error) {
	now := time.Now().UTC()
	return types.Session{
			ID:            "session-1",
			WorkspaceRoot: "/workspace",
			State:         types.SessionStateIdle,
			CreatedAt:     now,
			UpdatedAt:     now,
		}, types.ContextHead{
			ID:        "head-1",
			SessionID: "session-1",
		}, false, nil
}

func (bridgeTestStore) InsertTurn(context.Context, types.Turn) error {
	return nil
}

type bridgeTestManager struct{}

func (bridgeTestManager) RegisterSession(types.Session) {}

func (bridgeTestManager) UpdateSession(types.Session) bool {
	return true
}

func (bridgeTestManager) SubmitTurn(_ context.Context, _ string, in session.SubmitTurnInput) (string, error) {
	return in.Turn.ID, nil
}

type cancelingWaiter struct {
	cancel context.CancelFunc
}

func (w cancelingWaiter) WaitParentReplyCommitted(ctx context.Context, _, _ string) (types.ParentReplyCommittedPayload, error) {
	w.cancel()
	<-ctx.Done()
	return types.ParentReplyCommittedPayload{}, ctx.Err()
}

func (w cancelingWaiter) WaitNextParentReplyCommitted(ctx context.Context, _ string, _ map[string]struct{}) (types.ParentReplyCommittedPayload, error) {
	<-ctx.Done()
	return types.ParentReplyCommittedPayload{}, ctx.Err()
}

type scriptedWaiter struct {
	first     types.ParentReplyCommittedPayload
	next      chan types.ParentReplyCommittedPayload
	firstTurn chan string
}

func (w scriptedWaiter) WaitParentReplyCommitted(_ context.Context, sessionID, turnID string) (types.ParentReplyCommittedPayload, error) {
	payload := w.first
	payload.SessionID = sessionID
	payload.TurnID = turnID
	if w.firstTurn != nil {
		w.firstTurn <- turnID
	}
	return payload, nil
}

func (w scriptedWaiter) WaitNextParentReplyCommitted(ctx context.Context, _ string, _ map[string]struct{}) (types.ParentReplyCommittedPayload, error) {
	select {
	case payload := <-w.next:
		return payload, nil
	case <-ctx.Done():
		return types.ParentReplyCommittedPayload{}, ctx.Err()
	}
}

type timeoutThenNextWaiter struct {
	next chan types.ParentReplyCommittedPayload
}

func (w timeoutThenNextWaiter) WaitParentReplyCommitted(ctx context.Context, _, _ string) (types.ParentReplyCommittedPayload, error) {
	<-ctx.Done()
	return types.ParentReplyCommittedPayload{}, ctx.Err()
}

func (w timeoutThenNextWaiter) WaitNextParentReplyCommitted(ctx context.Context, _ string, seen map[string]struct{}) (types.ParentReplyCommittedPayload, error) {
	for {
		select {
		case payload := <-w.next:
			if key := parentReplyKey(payload); key != "" {
				if _, ok := seen[key]; ok {
					continue
				}
			}
			return payload, nil
		case <-ctx.Done():
			return types.ParentReplyCommittedPayload{}, ctx.Err()
		}
	}
}

type recordingPoster struct {
	mu       sync.Mutex
	messages []discordOutboundMessage
}

func (p *recordingPoster) PostMessage(_ context.Context, _ string, msg discordOutboundMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, msg)
	return nil
}

func (p *recordingPoster) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}

func TestIsDiscordBackgroundReportReply(t *testing.T) {
	base := types.ParentReplyCommittedPayload{
		WorkspaceRoot: "/workspace",
		TurnKind:      types.TurnKindReportBatch,
		Text:          "summary",
	}
	if !isDiscordBackgroundReportReply(base, "/workspace") {
		t.Fatal("background child report reply should be forwarded")
	}

	withSourceTurn := base
	withSourceTurn.SourceParentTurnIDs = []string{"turn_1"}
	if isDiscordBackgroundReportReply(withSourceTurn, "/workspace") {
		t.Fatal("chat-bound child report reply should be handled by the original ingress watcher")
	}

	userTurn := base
	userTurn.TurnKind = types.TurnKindUserMessage
	if isDiscordBackgroundReportReply(userTurn, "/workspace") {
		t.Fatal("normal user replies should not be forwarded by background reporter")
	}

	otherWorkspace := base
	otherWorkspace.WorkspaceRoot = "/other"
	if isDiscordBackgroundReportReply(otherWorkspace, "/workspace") {
		t.Fatal("other workspace replies should not be forwarded")
	}
}

func (p *recordingPoster) texts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.messages))
	for _, msg := range p.messages {
		out = append(out, msg.Content)
	}
	return out
}

func TestBridgeMarksIngressCancelledWithoutPostingRuntimeFailureOnShutdown(t *testing.T) {
	db := openDiscordBridgeTestDB(t)
	defer db.Close()
	state := NewStateStore(db)
	ctx, cancel := context.WithCancel(context.Background())
	poster := &recordingPoster{}
	bridge := &Bridge{
		state:   state,
		store:   bridgeTestStore{},
		manager: bridgeTestManager{},
		waiter:  cancelingWaiter{cancel: cancel},
		replies: poster,
		cfg: WorkspaceBinding{
			ReplyWaitTimeoutSeconds: 120,
		},
	}

	err := bridge.HandleAcceptedMessage(ctx, AcceptedMessage{
		DiscordMessageID: "discord-1",
		GuildID:          "guild-1",
		ChannelID:        "channel-1",
		AuthorID:         "user-1",
		WorkspaceRoot:    "/workspace",
		CleanedText:      "hello",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("HandleAcceptedMessage error = %v, want context.Canceled", err)
	}
	if poster.count() != 0 {
		t.Fatalf("PostMessage count = %d, want 0", poster.count())
	}

	rec, ok, err := state.GetDiscordIngress(context.Background(), "discord-1")
	if err != nil {
		t.Fatalf("GetDiscordIngress: %v", err)
	}
	if !ok {
		t.Fatal("discord ingress row was not recorded")
	}
	if rec.Status != discordIngressStatusCancelled {
		t.Fatalf("status = %q, want %q", rec.Status, discordIngressStatusCancelled)
	}
}

func TestBridgeTimeoutKeepsWatchingForLateReport(t *testing.T) {
	db := openDiscordBridgeTestDB(t)
	defer db.Close()
	state := NewStateStore(db)
	poster := &recordingPoster{}
	next := make(chan types.ParentReplyCommittedPayload, 1)
	bridge := &Bridge{
		state:   state,
		store:   bridgeTestStore{},
		manager: bridgeTestManager{},
		waiter:  timeoutThenNextWaiter{next: next},
		replies: poster,
		cfg: WorkspaceBinding{
			PostAcknowledgement: true,
		},
		replyWaitTimeout: 20 * time.Millisecond,
		backgroundCtx:    context.Background(),
	}

	err := bridge.HandleAcceptedMessage(context.Background(), AcceptedMessage{
		DiscordMessageID: "discord-1",
		GuildID:          "guild-1",
		ChannelID:        "channel-1",
		AuthorID:         "user-1",
		WorkspaceRoot:    "/workspace",
		CleanedText:      "delegate this",
	})
	if err != nil {
		t.Fatalf("HandleAcceptedMessage: %v", err)
	}

	rec, ok, err := state.GetDiscordIngress(context.Background(), "discord-1")
	if err != nil {
		t.Fatalf("GetDiscordIngress: %v", err)
	}
	if !ok {
		t.Fatal("discord ingress row was not recorded")
	}
	if rec.Status != discordIngressStatusReplyWaitExpired {
		t.Fatalf("status = %q, want %q", rec.Status, discordIngressStatusReplyWaitExpired)
	}
	if texts := poster.texts(); len(texts) != 1 || texts[0] != acknowledgementReplyText {
		t.Fatalf("posted messages = %#v, want only acknowledgement before late report", texts)
	}

	next <- types.ParentReplyCommittedPayload{
		WorkspaceRoot:       "/workspace",
		SessionID:           "session-1",
		TurnID:              "turn-child-report",
		TurnKind:            types.TurnKindReportBatch,
		SourceParentTurnIDs: []string{rec.SesameTurnID},
		ItemID:              2,
		Text:                "child report summary",
	}

	deadline := time.After(500 * time.Millisecond)
	for poster.count() < 2 {
		select {
		case <-deadline:
			t.Fatalf("PostMessage texts = %#v, want acknowledgement and late child report", poster.texts())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if texts := poster.texts(); texts[1] != "child report summary" {
		t.Fatalf("late reply text = %q, want child report summary", texts[1])
	}
}

func TestBridgePostsAdditionalParentReplies(t *testing.T) {
	db := openDiscordBridgeTestDB(t)
	defer db.Close()
	state := NewStateStore(db)
	poster := &recordingPoster{}
	next := make(chan types.ParentReplyCommittedPayload, 1)
	firstTurn := make(chan string, 1)
	bridge := &Bridge{
		state:   state,
		store:   bridgeTestStore{},
		manager: bridgeTestManager{},
		waiter: scriptedWaiter{
			first: types.ParentReplyCommittedPayload{
				WorkspaceRoot: "/workspace",
				SessionID:     "session-1",
				TurnID:        "turn-initial",
				ItemID:        1,
				Text:          "initial reply",
			},
			next:      next,
			firstTurn: firstTurn,
		},
		replies:          poster,
		cfg:              WorkspaceBinding{},
		replyWaitTimeout: 50 * time.Millisecond,
		backgroundCtx:    context.Background(),
	}

	err := bridge.HandleAcceptedMessage(context.Background(), AcceptedMessage{
		DiscordMessageID: "discord-1",
		GuildID:          "guild-1",
		ChannelID:        "channel-1",
		AuthorID:         "user-1",
		WorkspaceRoot:    "/workspace",
		CleanedText:      "hello",
	})
	if err != nil {
		t.Fatalf("HandleAcceptedMessage: %v", err)
	}

	parentTurnID := <-firstTurn
	next <- types.ParentReplyCommittedPayload{
		WorkspaceRoot:       "/workspace",
		SessionID:           "session-1",
		TurnID:              "turn-followup",
		TurnKind:            types.TurnKindReportBatch,
		SourceParentTurnIDs: []string{parentTurnID},
		ItemID:              2,
		Text:                "follow-up reply",
	}

	deadline := time.After(500 * time.Millisecond)
	for poster.count() < 2 {
		select {
		case <-deadline:
			t.Fatalf("PostMessage count = %d, want at least 2", poster.count())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestBridgeIgnoresUnrelatedAdditionalParentReplies(t *testing.T) {
	db := openDiscordBridgeTestDB(t)
	defer db.Close()
	state := NewStateStore(db)
	poster := &recordingPoster{}
	next := make(chan types.ParentReplyCommittedPayload, 2)
	bridge := &Bridge{
		state:   state,
		store:   bridgeTestStore{},
		manager: bridgeTestManager{},
		waiter: scriptedWaiter{
			first: types.ParentReplyCommittedPayload{
				WorkspaceRoot: "/workspace",
				SessionID:     "session-1",
				ItemID:        1,
				Text:          "initial reply",
			},
			next: next,
		},
		replies:          poster,
		cfg:              WorkspaceBinding{},
		replyWaitTimeout: 25 * time.Millisecond,
		backgroundCtx:    context.Background(),
	}

	err := bridge.HandleAcceptedMessage(context.Background(), AcceptedMessage{
		DiscordMessageID: "discord-1",
		GuildID:          "guild-1",
		ChannelID:        "channel-1",
		AuthorID:         "user-1",
		WorkspaceRoot:    "/workspace",
		CleanedText:      "hello",
	})
	if err != nil {
		t.Fatalf("HandleAcceptedMessage: %v", err)
	}

	next <- types.ParentReplyCommittedPayload{
		WorkspaceRoot:       "/workspace",
		SessionID:           "session-1",
		TurnID:              "turn-unrelated",
		TurnKind:            types.TurnKindReportBatch,
		SourceParentTurnIDs: []string{"different-parent-turn"},
		ItemID:              2,
		Text:                "unrelated child report",
	}
	next <- types.ParentReplyCommittedPayload{
		WorkspaceRoot: "/workspace",
		SessionID:     "session-1",
		TurnID:        "turn-user",
		TurnKind:      types.TurnKindUserMessage,
		ItemID:        3,
		Text:          "later user reply",
	}

	time.Sleep(100 * time.Millisecond)
	if poster.count() != 1 {
		t.Fatalf("PostMessage count = %d, want only initial reply", poster.count())
	}
}

func openDiscordBridgeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	_, err = db.Exec(`
		create table discord_ingress (
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
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
