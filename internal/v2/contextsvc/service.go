package contextsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/v2/agent"
	"go-agent/internal/v2/contracts"
)

const (
	previewContextBlockLimit = 20
	previewMemoryLimit       = 8
	previewReportLimit       = 8
)

var (
	ErrInvalidInput = errors.New("invalid context input")
	ErrNotFound     = errors.New("context not found")
)

type MemoryRecall interface {
	Recall(ctx context.Context, workspaceRoot, query string, limit int) ([]contracts.Memory, error)
}

type Service struct {
	store  contracts.Store
	memory MemoryRecall
	now    func() time.Time
}

func New(store contracts.Store, memory MemoryRecall) *Service {
	return &Service{
		store:  store,
		memory: memory,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

type PreviewInput struct {
	SessionID        string
	DefaultSessionID string
	SystemPrompt     string
}

type PreviewResponse struct {
	SessionID     string         `json:"session_id"`
	WorkspaceRoot string         `json:"workspace_root"`
	GeneratedAt   time.Time      `json:"generated_at"`
	ApproxTokens  int            `json:"approx_tokens"`
	Prompt        []PromptItem   `json:"prompt"`
	Blocks        []PreviewBlock `json:"blocks"`
}

type PromptItem struct {
	Role           string `json:"role"`
	SourceRef      string `json:"source_ref"`
	ContentPreview string `json:"content_preview"`
	ApproxTokens   int    `json:"approx_tokens"`
}

type PreviewBlock struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Owner           string  `json:"owner"`
	Visibility      string  `json:"visibility"`
	SourceRef       string  `json:"source_ref"`
	Status          string  `json:"status"`
	Reason          string  `json:"reason,omitempty"`
	Title           string  `json:"title,omitempty"`
	Summary         string  `json:"summary,omitempty"`
	ImportanceScore float64 `json:"importance_score,omitempty"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
}

type BlockInput struct {
	ID              *string    `json:"id,omitempty"`
	WorkspaceRoot   *string    `json:"workspace_root,omitempty"`
	Type            *string    `json:"type,omitempty"`
	Owner           *string    `json:"owner,omitempty"`
	Visibility      *string    `json:"visibility,omitempty"`
	SourceRef       *string    `json:"source_ref,omitempty"`
	Title           *string    `json:"title,omitempty"`
	Summary         *string    `json:"summary,omitempty"`
	Evidence        *string    `json:"evidence,omitempty"`
	Confidence      *float64   `json:"confidence,omitempty"`
	ImportanceScore *float64   `json:"importance_score,omitempty"`
	ExpiryPolicy    *string    `json:"expiry_policy,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

func (s *Service) Preview(ctx context.Context, input PreviewInput) (PreviewResponse, error) {
	sessionID := firstNonEmpty(input.SessionID, input.DefaultSessionID)
	if strings.TrimSpace(sessionID) == "" {
		return PreviewResponse{}, fmt.Errorf("%w: session_id is required", ErrInvalidInput)
	}
	session, err := s.store.Sessions().Get(ctx, sessionID)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("%w: session %q: %v", ErrNotFound, sessionID, err)
	}

	systemPrompt := firstNonEmpty(input.SystemPrompt, session.SystemPrompt)
	projectState, hasProjectState, err := s.store.ProjectStates().Get(ctx, session.WorkspaceRoot)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load project state: %w", err)
	}
	instructions := agent.BuildInstructions(systemPrompt, projectState.Summary)

	messages, err := s.store.Messages().List(ctx, sessionID, contracts.MessageListOptions{})
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load messages: %w", err)
	}
	promptMessages := agent.BuildContext(instructions, messages, nil, 0)
	promptItems := make([]PromptItem, 0, len(promptMessages))
	blocks := make([]PreviewBlock, 0, 4)
	totalTokens := 0
	for _, msg := range promptMessages {
		sourceRef := messageSourceRef(msg)
		tokens := agent.ApproximateMessageTokens([]contracts.Message{msg})
		totalTokens += tokens
		promptItems = append(promptItems, PromptItem{
			Role:           msg.Role,
			SourceRef:      sourceRef,
			ContentPreview: previewText(msg.Content),
			ApproxTokens:   tokens,
		})
		if msg.Role == "system" && msg.ID <= 0 {
			blocks = append(blocks, PreviewBlock{
				ID:         "system_prompt",
				Type:       "constraint",
				Owner:      "workspace",
				Visibility: "global",
				SourceRef:  "system_prompt",
				Status:     "included",
				Title:      "System instructions",
				Summary:    previewText(msg.Content),
			})
		}
	}
	if hasProjectState && strings.TrimSpace(projectState.Summary) != "" {
		blocks = append(blocks, PreviewBlock{
			ID:              "project_state",
			Type:            "fact",
			Owner:           "workspace",
			Visibility:      "global",
			SourceRef:       "project_state:" + session.WorkspaceRoot,
			Status:          "included",
			Title:           "Project State",
			Summary:         previewText(projectState.Summary),
			ImportanceScore: 1,
			UpdatedAt:       formatTime(projectState.UpdatedAt),
		})
	}

	indexBlocks, err := s.contextIndexBlocks(ctx, session.WorkspaceRoot, previewContextBlockLimit)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load context blocks: %w", err)
	}
	blocks = append(blocks, indexBlocks...)
	memoryBlocks, err := s.memoryBlocks(ctx, session.WorkspaceRoot, previewMemoryLimit)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load memory: %w", err)
	}
	blocks = append(blocks, memoryBlocks...)
	reportBlocks, err := s.reportBlocks(ctx, sessionID, previewReportLimit)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load reports: %w", err)
	}
	blocks = append(blocks, reportBlocks...)

	return PreviewResponse{
		SessionID:     session.ID,
		WorkspaceRoot: session.WorkspaceRoot,
		GeneratedAt:   s.now(),
		ApproxTokens:  totalTokens,
		Prompt:        promptItems,
		Blocks:        blocks,
	}, nil
}

func (s *Service) ListBlocks(ctx context.Context, workspaceRoot string, opts contracts.ContextBlockListOptions) ([]contracts.ContextBlock, error) {
	blocks, err := s.store.ContextBlocks().ListByWorkspace(ctx, strings.TrimSpace(workspaceRoot), opts)
	if err != nil {
		return nil, err
	}
	if blocks == nil {
		blocks = []contracts.ContextBlock{}
	}
	return blocks, nil
}

func (s *Service) CreateBlock(ctx context.Context, workspaceRoot string, input BlockInput, newID func(string) string) (contracts.ContextBlock, error) {
	now := s.now()
	id := newID("ctxblk")
	block := contracts.ContextBlock{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		Type:          "fact",
		Owner:         "workspace",
		Visibility:    "global",
		Confidence:    1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	block = applyBlockInput(block, input)
	block.ID = id
	block.WorkspaceRoot = workspaceRoot
	block = normalizeBlock(block)
	if err := validateBlock(block); err != nil {
		return contracts.ContextBlock{}, err
	}
	if err := s.store.ContextBlocks().Create(ctx, block); err != nil {
		return contracts.ContextBlock{}, err
	}
	return s.store.ContextBlocks().Get(ctx, block.ID)
}

func (s *Service) UpdateBlock(ctx context.Context, id string, input BlockInput) (contracts.ContextBlock, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return contracts.ContextBlock{}, fmt.Errorf("%w: context block id is required", ErrInvalidInput)
	}
	existing, err := s.store.ContextBlocks().Get(ctx, id)
	if err != nil {
		return contracts.ContextBlock{}, fmt.Errorf("%w: context block %q: %v", ErrNotFound, id, err)
	}
	block := existing
	block.UpdatedAt = s.now()
	block = applyBlockInput(block, input)
	block.ID = id
	block.WorkspaceRoot = existing.WorkspaceRoot
	block.CreatedAt = existing.CreatedAt
	block = normalizeBlock(block)
	if err := validateBlock(block); err != nil {
		return contracts.ContextBlock{}, err
	}
	if err := s.store.ContextBlocks().Update(ctx, block); err != nil {
		return contracts.ContextBlock{}, err
	}
	return s.store.ContextBlocks().Get(ctx, id)
}

func (s *Service) DeleteBlock(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: context block id is required", ErrInvalidInput)
	}
	return s.store.ContextBlocks().Delete(ctx, id)
}

func (s *Service) contextIndexBlocks(ctx context.Context, workspaceRoot string, limit int) ([]PreviewBlock, error) {
	contextBlocks, err := s.store.ContextBlocks().ListByWorkspace(ctx, workspaceRoot, contracts.ContextBlockListOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	now := s.now()
	blocks := make([]PreviewBlock, 0, len(contextBlocks))
	for _, block := range contextBlocks {
		status := "available"
		reason := "ContextBlock is indexed workspace context. It is visible to preview but not automatically injected until context selection is enabled."
		if block.ExpiresAt != nil && !block.ExpiresAt.After(now) {
			status = "excluded"
			reason = "ContextBlock is expired."
		}
		blocks = append(blocks, PreviewBlock{
			ID:              block.ID,
			Type:            block.Type,
			Owner:           block.Owner,
			Visibility:      block.Visibility,
			SourceRef:       firstNonEmpty(block.SourceRef, "context_block:"+block.ID),
			Status:          status,
			Reason:          reason,
			Title:           block.Title,
			Summary:         previewText(firstNonEmpty(block.Summary, block.Evidence)),
			ImportanceScore: block.ImportanceScore,
			UpdatedAt:       formatTime(block.UpdatedAt),
		})
	}
	return blocks, nil
}

func (s *Service) memoryBlocks(ctx context.Context, workspaceRoot string, limit int) ([]PreviewBlock, error) {
	var memories []contracts.Memory
	var err error
	if s.memory != nil {
		memories, err = s.memory.Recall(ctx, workspaceRoot, "", limit)
	} else {
		memories, err = s.store.Memories().ListByWorkspace(ctx, workspaceRoot, limit)
	}
	if err != nil {
		return nil, err
	}
	blocks := make([]PreviewBlock, 0, len(memories))
	for _, memory := range memories {
		blocks = append(blocks, PreviewBlock{
			ID:              memory.ID,
			Type:            "memory",
			Owner:           "workspace",
			Visibility:      "global",
			SourceRef:       firstNonEmpty(memory.Source, "memory:"+memory.ID),
			Status:          "available",
			Reason:          "Memory is durable workspace context but is not injected into the prompt unless selected by a later context policy or tool flow.",
			Title:           memory.Kind,
			Summary:         previewText(memory.Content),
			ImportanceScore: memory.Confidence,
			UpdatedAt:       formatTime(memory.UpdatedAt),
		})
	}
	return blocks, nil
}

func (s *Service) reportBlocks(ctx context.Context, sessionID string, limit int) ([]PreviewBlock, error) {
	reports, err := s.store.Reports().ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	start := 0
	if limit > 0 && len(reports) > limit {
		start = len(reports) - limit
	}
	blocks := make([]PreviewBlock, 0, len(reports)-start)
	for i := len(reports) - 1; i >= start; i-- {
		report := reports[i]
		reason := "Delivered reports are represented in session history; report records stay available for audit."
		if !report.Delivered {
			reason = "Report is queued for delivery and is not part of the prompt until the report batch turn runs."
		}
		blocks = append(blocks, PreviewBlock{
			ID:         report.ID,
			Type:       "report",
			Owner:      "main_session",
			Visibility: "session",
			SourceRef:  report.SourceKind + ":" + report.SourceID,
			Status:     "available",
			Reason:     reason,
			Title:      report.Title,
			Summary:    previewText(report.Summary),
			UpdatedAt:  formatTime(report.CreatedAt),
		})
	}
	return blocks, nil
}

func applyBlockInput(block contracts.ContextBlock, input BlockInput) contracts.ContextBlock {
	if input.ID != nil {
		block.ID = *input.ID
	}
	if input.WorkspaceRoot != nil {
		block.WorkspaceRoot = *input.WorkspaceRoot
	}
	if input.Type != nil {
		block.Type = *input.Type
	}
	if input.Owner != nil {
		block.Owner = *input.Owner
	}
	if input.Visibility != nil {
		block.Visibility = *input.Visibility
	}
	if input.SourceRef != nil {
		block.SourceRef = *input.SourceRef
	}
	if input.Title != nil {
		block.Title = *input.Title
	}
	if input.Summary != nil {
		block.Summary = *input.Summary
	}
	if input.Evidence != nil {
		block.Evidence = *input.Evidence
	}
	if input.Confidence != nil {
		block.Confidence = *input.Confidence
	}
	if input.ImportanceScore != nil {
		block.ImportanceScore = *input.ImportanceScore
	}
	if input.ExpiryPolicy != nil {
		block.ExpiryPolicy = *input.ExpiryPolicy
	}
	if input.ExpiresAt != nil {
		block.ExpiresAt = input.ExpiresAt
	}
	return block
}

func normalizeBlock(block contracts.ContextBlock) contracts.ContextBlock {
	block.ID = strings.TrimSpace(block.ID)
	block.WorkspaceRoot = strings.TrimSpace(block.WorkspaceRoot)
	block.Type = firstNonEmpty(block.Type, "fact")
	block.Owner = firstNonEmpty(block.Owner, "workspace")
	block.Visibility = firstNonEmpty(block.Visibility, "global")
	block.SourceRef = strings.TrimSpace(block.SourceRef)
	block.Title = strings.TrimSpace(block.Title)
	block.Summary = strings.TrimSpace(block.Summary)
	block.Evidence = strings.TrimSpace(block.Evidence)
	block.ExpiryPolicy = strings.TrimSpace(block.ExpiryPolicy)
	return block
}

func validateBlock(block contracts.ContextBlock) error {
	if strings.TrimSpace(block.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(block.WorkspaceRoot) == "" {
		return fmt.Errorf("%w: workspace_root is required", ErrInvalidInput)
	}
	if strings.TrimSpace(block.Summary) == "" && strings.TrimSpace(block.Evidence) == "" {
		return fmt.Errorf("%w: summary or evidence is required", ErrInvalidInput)
	}
	return nil
}

func messageSourceRef(msg contracts.Message) string {
	if msg.ID <= 0 {
		return "system_prompt"
	}
	return fmt.Sprintf("message:%d", msg.ID)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func previewText(text string) string {
	text = strings.TrimSpace(text)
	const maxRunes = 240
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
