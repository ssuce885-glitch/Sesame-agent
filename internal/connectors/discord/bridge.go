package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/session"
	"go-agent/internal/types"
)

const (
	discordIngressStatusAccepted         = "accepted"
	discordIngressStatusAwaitingReply    = "awaiting_reply"
	discordIngressStatusReplyPosted      = "reply_posted"
	discordIngressStatusFinalPosted      = "final_posted"
	discordIngressStatusReplyWaitExpired = "reply_wait_expired"
	discordIngressStatusFinalPostFailed  = "final_post_failed"
	discordIngressStatusRuntimeFailed    = "runtime_failed_without_parent_reply"
	discordIngressStatusSubmitFailed     = "submit_failed"
	discordIngressStatusCancelled        = "cancelled"

	defaultReplyWaitTimeout    = 120 * time.Second
	defaultFollowupWaitTimeout = 30 * time.Minute
	statusUpdateTimeout        = 5 * time.Second
)

type bridgeRuntimeStore interface {
	EnsureRoleSession(ctx context.Context, workspaceRoot string, role types.SessionRole) (types.Session, types.ContextHead, bool, error)
	InsertTurn(ctx context.Context, turn types.Turn) error
}

type bridgeRuntimeManager interface {
	RegisterSession(types.Session)
	UpdateSession(types.Session) bool
	SubmitTurn(ctx context.Context, sessionID string, in session.SubmitTurnInput) (string, error)
}

type parentReplyWaiter interface {
	WaitParentReplyCommitted(ctx context.Context, sessionID, turnID string) (types.ParentReplyCommittedPayload, error)
	WaitNextParentReplyCommitted(ctx context.Context, sessionID string, seen map[string]struct{}) (types.ParentReplyCommittedPayload, error)
}

type AcceptedMessage struct {
	DiscordMessageID string
	GuildID          string
	ChannelID        string
	AuthorID         string
	WorkspaceRoot    string
	CleanedText      string
}

type Bridge struct {
	state   *StateStore
	store   bridgeRuntimeStore
	manager bridgeRuntimeManager
	waiter  parentReplyWaiter
	replies discordReplyPoster
	cfg     WorkspaceBinding

	replyWaitTimeout time.Duration
	now              func() time.Time
	backgroundCtx    context.Context
}

func (b *Bridge) HandleAcceptedMessage(ctx context.Context, msg AcceptedMessage) error {
	if err := b.validateDependencies(); err != nil {
		return err
	}

	msg = normalizeAcceptedMessage(msg)
	if err := b.state.UpsertDiscordIngress(ctx, IngressRecord{
		DiscordMessageID: msg.DiscordMessageID,
		GuildID:          msg.GuildID,
		ChannelID:        msg.ChannelID,
		AuthorID:         msg.AuthorID,
		WorkspaceRoot:    msg.WorkspaceRoot,
		Status:           discordIngressStatusAccepted,
	}); err != nil {
		return err
	}

	sessionRow, head, _, err := b.store.EnsureRoleSession(ctx, msg.WorkspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		return b.failSubmit(ctx, msg, err)
	}
	if !b.manager.UpdateSession(sessionRow) {
		b.manager.RegisterSession(sessionRow)
	}

	turn := b.newTurn(sessionRow.ID, head.ID, msg.CleanedText)
	if err := b.store.InsertTurn(ctx, turn); err != nil {
		return b.failSubmit(ctx, msg, err)
	}

	submittedTurnID, err := b.manager.SubmitTurn(ctx, sessionRow.ID, session.SubmitTurnInput{Turn: turn})
	if err != nil {
		return b.failSubmit(ctx, msg, err)
	}
	turnID := strings.TrimSpace(submittedTurnID)
	if turnID == "" {
		turnID = turn.ID
	}

	if err := b.state.SetDiscordIngressTurnID(ctx, msg.DiscordMessageID, turnID); err != nil {
		return b.failSubmit(ctx, msg, err)
	}
	if err := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusAwaitingReply, ""); err != nil {
		return err
	}

	if b.cfg.PostAcknowledgement {
		_ = postAcknowledgement(ctx, b.replies, msg.ChannelID, msg.DiscordMessageID)
	}

	payload, err := b.waitForParentReplyCommitted(ctx, sessionRow.ID, turnID)
	if err != nil {
		return b.handleReplyWaitError(ctx, msg, sessionRow.ID, turnID, err)
	}
	if err := validateCommittedPayload(payload, msg.WorkspaceRoot, sessionRow.ID, turnID); err != nil {
		return b.handleReplyWaitError(ctx, msg, sessionRow.ID, turnID, err)
	}

	if err := postFinalReply(ctx, b.replies, msg.ChannelID, msg.DiscordMessageID, payload.Text, b.cfg); err != nil {
		statusErr := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusFinalPostFailed, err.Error())
		return joinErrors(err, statusErr)
	}

	seen := map[string]struct{}{parentReplyKey(payload): {}}
	if err := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusReplyPosted, ""); err != nil {
		return err
	}
	b.watchAdditionalParentReplies(msg, sessionRow.ID, turnID, seen)
	return nil
}

func (b *Bridge) failSubmit(ctx context.Context, msg AcceptedMessage, cause error) error {
	statusErr := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusSubmitFailed, cause.Error())
	replyErr := postGenericReply(ctx, b.replies, msg.ChannelID, msg.DiscordMessageID, genericReplySubmitFailed)
	return joinErrors(cause, statusErr, replyErr)
}

func (b *Bridge) handleReplyWaitError(ctx context.Context, msg AcceptedMessage, sessionID, turnID string, waitErr error) error {
	if errors.Is(waitErr, context.DeadlineExceeded) {
		statusErr := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusReplyWaitExpired, waitErr.Error())
		b.watchLateParentReplies(msg, sessionID, turnID, map[string]struct{}{}, true)
		return statusErr
	}
	if errors.Is(waitErr, context.Canceled) {
		statusErr := b.setIngressStatusAfterCancel(msg.DiscordMessageID, discordIngressStatusCancelled, waitErr.Error())
		return joinErrors(waitErr, statusErr)
	}
	statusErr := b.state.SetDiscordIngressStatus(ctx, msg.DiscordMessageID, discordIngressStatusRuntimeFailed, waitErr.Error())
	replyErr := postGenericReply(ctx, b.replies, msg.ChannelID, msg.DiscordMessageID, genericReplyRuntime)
	return joinErrors(waitErr, statusErr, replyErr)
}

func (b *Bridge) setIngressStatusAfterCancel(discordMessageID, status, errorMessage string) error {
	if b.state == nil {
		return errors.New("discord bridge state store is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusUpdateTimeout)
	defer cancel()
	return b.state.SetDiscordIngressStatus(ctx, discordMessageID, status, errorMessage)
}

func (b *Bridge) watchAdditionalParentReplies(msg AcceptedMessage, sessionID, parentTurnID string, seen map[string]struct{}) {
	b.watchLateParentReplies(msg, sessionID, parentTurnID, seen, false)
}

func (b *Bridge) watchLateParentReplies(msg AcceptedMessage, sessionID, parentTurnID string, seen map[string]struct{}, includeOriginalTurn bool) {
	if b.waiter == nil || b.state == nil || b.replies == nil {
		return
	}
	waitCtx := b.backgroundCtx
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	timeout := b.resolveFollowupWaitTimeout()
	go func() {
		ctx, cancel := context.WithTimeout(waitCtx, timeout)
		defer cancel()
		if seen == nil {
			seen = map[string]struct{}{}
		}
		postedAny := !includeOriginalTurn
		for {
			payload, err := b.waiter.WaitNextParentReplyCommitted(ctx, sessionID, seen)
			if err != nil {
				if postedAny && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
					_ = b.state.SetDiscordIngressStatus(context.Background(), msg.DiscordMessageID, discordIngressStatusFinalPosted, "")
				}
				return
			}
			key := parentReplyKey(payload)
			if key != "" {
				seen[key] = struct{}{}
			}
			if !isDiscordRelevantLateReply(payload, msg.WorkspaceRoot, sessionID, parentTurnID, includeOriginalTurn) {
				continue
			}
			if err := postFinalReply(ctx, b.replies, msg.ChannelID, msg.DiscordMessageID, payload.Text, b.cfg); err != nil {
				_ = b.state.SetDiscordIngressStatus(context.Background(), msg.DiscordMessageID, discordIngressStatusFinalPostFailed, err.Error())
				return
			}
			postedAny = true
		}
	}()
}

func isDiscordRelevantLateReply(payload types.ParentReplyCommittedPayload, workspaceRoot, sessionID, parentTurnID string, includeOriginalTurn bool) bool {
	if includeOriginalTurn {
		if validateCommittedPayload(payload, workspaceRoot, sessionID, parentTurnID) == nil {
			return true
		}
	}
	return isDiscordFollowupReply(payload, workspaceRoot, parentTurnID)
}

func isDiscordFollowupReply(payload types.ParentReplyCommittedPayload, workspaceRoot, parentTurnID string) bool {
	if strings.TrimSpace(payload.WorkspaceRoot) != "" && strings.TrimSpace(payload.WorkspaceRoot) != strings.TrimSpace(workspaceRoot) {
		return false
	}
	if payload.TurnKind != types.TurnKindReportBatch {
		return false
	}
	parentTurnID = strings.TrimSpace(parentTurnID)
	if parentTurnID == "" {
		return false
	}
	for _, sourceTurnID := range payload.SourceParentTurnIDs {
		if strings.TrimSpace(sourceTurnID) == parentTurnID {
			return true
		}
	}
	return false
}

func parentReplyKey(payload types.ParentReplyCommittedPayload) string {
	turnID := strings.TrimSpace(payload.TurnID)
	if turnID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", turnID, payload.ItemID)
}

func validateCommittedPayload(payload types.ParentReplyCommittedPayload, workspaceRoot, sessionID, turnID string) error {
	if strings.TrimSpace(payload.TurnID) == "" {
		return errors.New("parent reply turn id is empty")
	}
	if strings.TrimSpace(payload.TurnID) != strings.TrimSpace(turnID) {
		return fmt.Errorf("parent reply turn mismatch: got %q, want %q", payload.TurnID, turnID)
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		return errors.New("parent reply session id is empty")
	}
	if strings.TrimSpace(payload.SessionID) != strings.TrimSpace(sessionID) {
		return fmt.Errorf("parent reply session mismatch: got %q, want %q", payload.SessionID, sessionID)
	}
	if strings.TrimSpace(payload.WorkspaceRoot) == "" {
		return errors.New("parent reply workspace root is empty")
	}
	if strings.TrimSpace(payload.WorkspaceRoot) != strings.TrimSpace(workspaceRoot) {
		return fmt.Errorf("parent reply workspace mismatch: got %q, want %q", payload.WorkspaceRoot, workspaceRoot)
	}
	return nil
}

func (b *Bridge) waitForParentReplyCommitted(ctx context.Context, sessionID, turnID string) (types.ParentReplyCommittedPayload, error) {
	timeout := b.resolveReplyWaitTimeout()
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return b.waiter.WaitParentReplyCommitted(waitCtx, sessionID, turnID)
}

func (b *Bridge) resolveReplyWaitTimeout() time.Duration {
	if b.replyWaitTimeout > 0 {
		return b.replyWaitTimeout
	}
	seconds := b.cfg.ReplyWaitTimeoutSeconds
	if seconds <= 0 {
		return defaultReplyWaitTimeout
	}
	return time.Duration(seconds) * time.Second
}

func (b *Bridge) resolveFollowupWaitTimeout() time.Duration {
	if b.replyWaitTimeout > 0 {
		return b.replyWaitTimeout
	}
	timeout := b.resolveReplyWaitTimeout() * 3
	if timeout < defaultFollowupWaitTimeout {
		return defaultFollowupWaitTimeout
	}
	return timeout
}

func (b *Bridge) newTurn(sessionID, contextHeadID, userMessage string) types.Turn {
	now := time.Now().UTC()
	if b.now != nil {
		now = b.now().UTC()
	}
	return types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     strings.TrimSpace(sessionID),
		ContextHeadID: strings.TrimSpace(contextHeadID),
		Kind:          types.TurnKindUserMessage,
		State:         types.TurnStateCreated,
		UserMessage:   strings.TrimSpace(userMessage),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (b *Bridge) validateDependencies() error {
	switch {
	case b.state == nil:
		return errors.New("discord bridge state store is not configured")
	case b.store == nil:
		return errors.New("discord bridge runtime store is not configured")
	case b.manager == nil:
		return errors.New("discord bridge runtime manager is not configured")
	case b.waiter == nil:
		return errors.New("discord bridge parent reply waiter is not configured")
	case b.replies == nil:
		return errors.New("discord bridge reply poster is not configured")
	default:
		return nil
	}
}

func normalizeAcceptedMessage(msg AcceptedMessage) AcceptedMessage {
	msg.DiscordMessageID = strings.TrimSpace(msg.DiscordMessageID)
	msg.GuildID = strings.TrimSpace(msg.GuildID)
	msg.ChannelID = strings.TrimSpace(msg.ChannelID)
	msg.AuthorID = strings.TrimSpace(msg.AuthorID)
	msg.WorkspaceRoot = strings.TrimSpace(msg.WorkspaceRoot)
	msg.CleanedText = strings.TrimSpace(msg.CleanedText)
	return msg
}

func joinErrors(errs ...error) error {
	var nonNil []error
	for _, err := range errs {
		if err != nil {
			nonNil = append(nonNil, err)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	return errors.Join(nonNil...)
}
