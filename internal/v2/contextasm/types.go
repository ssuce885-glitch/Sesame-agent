package contextasm

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidInput = errors.New("invalid contextasm input")

type ScopeKind string

const (
	ScopeMain ScopeKind = "main"
	ScopeRole ScopeKind = "role"
	ScopeTask ScopeKind = "task"
)

type ExecutionScope struct {
	Kind   ScopeKind `json:"kind"`
	RoleID string    `json:"role_id,omitempty"`
	TaskID string    `json:"task_id,omitempty"`
}

func (s ExecutionScope) normalized() ExecutionScope {
	s.Kind = ScopeKind(strings.TrimSpace(string(s.Kind)))
	s.RoleID = strings.TrimSpace(s.RoleID)
	s.TaskID = strings.TrimSpace(s.TaskID)
	return s
}

func (s ExecutionScope) Validate() error {
	s = s.normalized()
	switch s.Kind {
	case ScopeMain:
		if s.RoleID != "" || s.TaskID != "" {
			return fmt.Errorf("%w: main scope does not accept role_id or task_id", ErrInvalidInput)
		}
	case ScopeRole:
		if s.RoleID == "" {
			return fmt.Errorf("%w: role scope requires role_id", ErrInvalidInput)
		}
		if s.TaskID != "" {
			return fmt.Errorf("%w: role scope does not accept task_id", ErrInvalidInput)
		}
	case ScopeTask:
		if s.TaskID == "" {
			return fmt.Errorf("%w: task scope requires task_id", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported scope %q", ErrInvalidInput, s.Kind)
	}
	return nil
}

type SourceRef struct {
	Ref   string `json:"ref"`
	Label string `json:"label,omitempty"`
}

func (r SourceRef) normalized() SourceRef {
	r.Ref = strings.TrimSpace(r.Ref)
	r.Label = strings.TrimSpace(r.Label)
	return r
}

func (r SourceRef) Validate() error {
	r = r.normalized()
	if r.Ref == "" {
		return fmt.Errorf("%w: source ref is required", ErrInvalidInput)
	}
	return nil
}

type SourceBlock struct {
	ID            string      `json:"id"`
	Type          string      `json:"type"`
	Owner         string      `json:"owner"`
	Visibility    string      `json:"visibility"`
	Title         string      `json:"title,omitempty"`
	Content       string      `json:"content"`
	SourceRefs    []SourceRef `json:"source_refs,omitempty"`
	Importance    float64     `json:"importance,omitempty"`
	UpdatedAt     time.Time   `json:"updated_at,omitempty"`
	TokenEstimate int         `json:"token_estimate,omitempty"`
}

func (b SourceBlock) normalized() SourceBlock {
	b.ID = strings.TrimSpace(b.ID)
	b.Type = strings.TrimSpace(b.Type)
	b.Owner = strings.TrimSpace(b.Owner)
	b.Visibility = strings.TrimSpace(b.Visibility)
	b.Title = strings.TrimSpace(b.Title)
	b.Content = strings.TrimSpace(b.Content)
	for i := range b.SourceRefs {
		b.SourceRefs[i] = b.SourceRefs[i].normalized()
	}
	return b
}

func (b SourceBlock) Validate() error {
	b = b.normalized()
	if b.ID == "" {
		return fmt.Errorf("%w: source block id is required", ErrInvalidInput)
	}
	if b.Type == "" {
		return fmt.Errorf("%w: source block %q type is required", ErrInvalidInput, b.ID)
	}
	if b.Owner == "" {
		return fmt.Errorf("%w: source block %q owner is required", ErrInvalidInput, b.ID)
	}
	if b.Visibility == "" {
		return fmt.Errorf("%w: source block %q visibility is required", ErrInvalidInput, b.ID)
	}
	if b.Content == "" {
		return fmt.Errorf("%w: source block %q content is required", ErrInvalidInput, b.ID)
	}
	for _, ref := range b.SourceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("%w: source block %q has invalid source ref: %v", ErrInvalidInput, b.ID, err)
		}
	}
	return nil
}

type Selection struct {
	Block       SourceBlock `json:"block"`
	WhySelected string      `json:"why_selected"`
}

type IncludedBlock struct {
	Block         SourceBlock `json:"block"`
	WhySelected   string      `json:"why_selected"`
	TokenEstimate int         `json:"token_estimate"`
}

type InstructionSource string

const (
	InstructionSourceGlobalSystem InstructionSource = "global_system"
	InstructionSourceAgents       InstructionSource = "agents_md"
	InstructionSourceCurrentUser  InstructionSource = "current_user"
	InstructionSourceTaskPrompt   InstructionSource = "task_prompt"
	InstructionSourceRolePrompt   InstructionSource = "role_prompt"
	InstructionSourceSkill        InstructionSource = "skill"
)

type InstructionConflict struct {
	DurableSource       InstructionSource `json:"durable_source"`
	OverrideSource      InstructionSource `json:"override_source"`
	Subject             string            `json:"subject"`
	Resolution          string            `json:"resolution"`
	SuggestAgentsUpdate bool              `json:"suggest_agents_update"`
	Note                string            `json:"note,omitempty"`
}

func (c InstructionConflict) normalized() InstructionConflict {
	c.DurableSource = InstructionSource(strings.TrimSpace(string(c.DurableSource)))
	c.OverrideSource = InstructionSource(strings.TrimSpace(string(c.OverrideSource)))
	c.Subject = strings.TrimSpace(c.Subject)
	c.Resolution = strings.TrimSpace(c.Resolution)
	c.Note = strings.TrimSpace(c.Note)
	return c
}

func (c InstructionConflict) Validate() error {
	c = c.normalized()
	if c.DurableSource == "" {
		return fmt.Errorf("%w: durable_source is required", ErrInvalidInput)
	}
	if c.OverrideSource == "" {
		return fmt.Errorf("%w: override_source is required", ErrInvalidInput)
	}
	if c.Subject == "" {
		return fmt.Errorf("%w: conflict subject is required", ErrInvalidInput)
	}
	if c.Resolution == "" {
		return fmt.Errorf("%w: conflict resolution is required", ErrInvalidInput)
	}
	return nil
}

func NewTurnOverrideConflict(subject string, overrideSource InstructionSource, note string) (InstructionConflict, error) {
	subject = strings.TrimSpace(subject)
	overrideSource = InstructionSource(strings.TrimSpace(string(overrideSource)))
	note = strings.TrimSpace(note)
	if subject == "" {
		return InstructionConflict{}, fmt.Errorf("%w: conflict subject is required", ErrInvalidInput)
	}
	switch overrideSource {
	case InstructionSourceCurrentUser, InstructionSourceTaskPrompt:
	default:
		return InstructionConflict{}, fmt.Errorf("%w: turn override must come from current_user or task_prompt, got %q", ErrInvalidInput, overrideSource)
	}
	return InstructionConflict{
		DurableSource:       InstructionSourceAgents,
		OverrideSource:      overrideSource,
		Subject:             subject,
		Resolution:          "turn_override",
		SuggestAgentsUpdate: true,
		Note:                note,
	}, nil
}

type PromptPackage struct {
	Scope              ExecutionScope        `json:"scope"`
	IncludedBlocks     []IncludedBlock       `json:"included_blocks"`
	SourceRefs         []SourceRef           `json:"source_refs"`
	TotalTokenEstimate int                   `json:"total_token_estimate"`
	Conflicts          []InstructionConflict `json:"conflicts,omitempty"`
}

type PackageInput struct {
	Scope      ExecutionScope        `json:"scope"`
	Selections []Selection           `json:"selections,omitempty"`
	Conflicts  []InstructionConflict `json:"conflicts,omitempty"`
}

type RuntimeItem struct {
	Summary     string   `json:"summary"`
	Status      string   `json:"status,omitempty"`
	Owner       string   `json:"owner"`
	Scope       string   `json:"scope"`
	SourceRef   string   `json:"source_ref"`
	RelatedRefs []string `json:"related_refs,omitempty"`
}

func (i RuntimeItem) normalized() RuntimeItem {
	i.Summary = strings.TrimSpace(i.Summary)
	i.Status = strings.TrimSpace(i.Status)
	i.Owner = strings.TrimSpace(i.Owner)
	i.Scope = strings.TrimSpace(i.Scope)
	i.SourceRef = strings.TrimSpace(i.SourceRef)
	for idx := range i.RelatedRefs {
		i.RelatedRefs[idx] = strings.TrimSpace(i.RelatedRefs[idx])
	}
	return i
}

func (i RuntimeItem) Validate() error {
	i = i.normalized()
	if i.Summary == "" {
		return fmt.Errorf("%w: runtime item summary is required", ErrInvalidInput)
	}
	if i.Owner == "" {
		return fmt.Errorf("%w: runtime item owner is required", ErrInvalidInput)
	}
	if i.Scope == "" {
		return fmt.Errorf("%w: runtime item scope is required", ErrInvalidInput)
	}
	if i.SourceRef == "" {
		return fmt.Errorf("%w: runtime item source_ref is required", ErrInvalidInput)
	}
	return nil
}

type RoleWorkstream struct {
	RoleID         string   `json:"role_id"`
	State          string   `json:"state"`
	Responsibility string   `json:"responsibility"`
	ActiveRefs     []string `json:"active_refs,omitempty"`
	LatestReport   string   `json:"latest_report,omitempty"`
	OpenLoop       string   `json:"open_loop,omitempty"`
	NextAction     string   `json:"next_action,omitempty"`
	Owner          string   `json:"owner"`
	Scope          string   `json:"scope"`
	SourceRef      string   `json:"source_ref"`
}

func (w RoleWorkstream) normalized() RoleWorkstream {
	w.RoleID = strings.TrimSpace(w.RoleID)
	w.State = strings.TrimSpace(w.State)
	w.Responsibility = strings.TrimSpace(w.Responsibility)
	w.LatestReport = strings.TrimSpace(w.LatestReport)
	w.OpenLoop = strings.TrimSpace(w.OpenLoop)
	w.NextAction = strings.TrimSpace(w.NextAction)
	w.Owner = strings.TrimSpace(w.Owner)
	w.Scope = strings.TrimSpace(w.Scope)
	w.SourceRef = strings.TrimSpace(w.SourceRef)
	for idx := range w.ActiveRefs {
		w.ActiveRefs[idx] = strings.TrimSpace(w.ActiveRefs[idx])
	}
	return w
}

func (w RoleWorkstream) Validate() error {
	w = w.normalized()
	if w.RoleID == "" {
		return fmt.Errorf("%w: role workstream role_id is required", ErrInvalidInput)
	}
	if w.State == "" {
		return fmt.Errorf("%w: role workstream %q state is required", ErrInvalidInput, w.RoleID)
	}
	if w.Responsibility == "" {
		return fmt.Errorf("%w: role workstream %q responsibility is required", ErrInvalidInput, w.RoleID)
	}
	if w.Owner == "" {
		return fmt.Errorf("%w: role workstream %q owner is required", ErrInvalidInput, w.RoleID)
	}
	if w.Scope == "" {
		return fmt.Errorf("%w: role workstream %q scope is required", ErrInvalidInput, w.RoleID)
	}
	if w.SourceRef == "" {
		return fmt.Errorf("%w: role workstream %q source_ref is required", ErrInvalidInput, w.RoleID)
	}
	return nil
}

type WorkspaceRuntimeStateInput struct {
	Objectives             []RuntimeItem    `json:"objectives,omitempty"`
	RoleWorkstreams        []RoleWorkstream `json:"role_workstreams,omitempty"`
	ActiveAutomations      []RuntimeItem    `json:"active_automations,omitempty"`
	ActiveWorkflowRuns     []RuntimeItem    `json:"active_workflow_runs,omitempty"`
	WorkspaceOpenLoops     []RuntimeItem    `json:"workspace_open_loops,omitempty"`
	RecentMaterialOutcomes []RuntimeItem    `json:"recent_material_outcomes,omitempty"`
	RuntimeHealth          []RuntimeItem    `json:"runtime_health,omitempty"`
	Watchpoints            []RuntimeItem    `json:"watchpoints,omitempty"`
	ImportantArtifacts     []RuntimeItem    `json:"important_artifacts,omitempty"`
}

type RoleRuntimeStateInput struct {
	RoleID                   string        `json:"role_id"`
	Responsibility           []RuntimeItem `json:"responsibility,omitempty"`
	OwnedAutomations         []RuntimeItem `json:"owned_automations,omitempty"`
	ActiveWork               []RuntimeItem `json:"active_work,omitempty"`
	OpenLoops                []RuntimeItem `json:"open_loops,omitempty"`
	RecentMaterialOutcomes   []RuntimeItem `json:"recent_material_outcomes,omitempty"`
	RelevantWorkspaceContext []RuntimeItem `json:"relevant_workspace_context,omitempty"`
	Watchpoints              []RuntimeItem `json:"watchpoints,omitempty"`
	ImportantArtifacts       []RuntimeItem `json:"important_artifacts,omitempty"`
}
