package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

type StatusResponse struct {
	Status               string `json:"status"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

type Client struct {
	baseURL        string
	httpClient     *http.Client
	contextBinding string
	sessionRole    types.SessionRole
}

func New(baseURL string, httpClient *http.Client) *Client {
	return NewWithContextBindingAndSessionRole(baseURL, httpClient, sessionbinding.DefaultContextBinding, types.SessionRoleMainParent)
}

func NewWithContextBinding(baseURL string, httpClient *http.Client, binding string) *Client {
	return NewWithContextBindingAndSessionRole(baseURL, httpClient, binding, types.SessionRoleMainParent)
}

func NewWithContextBindingAndSessionRole(baseURL string, httpClient *http.Client, binding string, role types.SessionRole) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		httpClient:     httpClient,
		contextBinding: sessionbinding.Normalize(binding),
		sessionRole:    sessionrole.Normalize(string(role)),
	}
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var out StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/status", nil, &out); err != nil {
		return StatusResponse{}, err
	}
	return out, nil
}

func (c *Client) EnsureSession(ctx context.Context, workspaceRoot string) (types.Session, error) {
	var out types.Session
	if err := c.doJSON(ctx, http.MethodPost, "/v1/session/ensure", types.EnsureSessionRequest{
		WorkspaceRoot: workspaceRoot,
		SessionRole:   string(c.sessionRole),
	}, &out); err != nil {
		return types.Session{}, err
	}
	return out, nil
}

func (c *Client) SubmitTurn(ctx context.Context, req types.SubmitTurnRequest) (types.Turn, error) {
	var out types.Turn
	if err := c.doJSON(ctx, http.MethodPost, "/v1/session/turns", req, &out); err != nil {
		return types.Turn{}, err
	}
	return out, nil
}

func (c *Client) InterruptTurn(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/session/interrupt", nil, nil)
}

func (c *Client) DecidePermission(ctx context.Context, req types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error) {
	var out types.PermissionDecisionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/permissions/decide", req, &out); err != nil {
		return types.PermissionDecisionResponse{}, err
	}
	return out, nil
}

func (c *Client) GetTimeline(ctx context.Context) (types.SessionTimelineResponse, error) {
	var out types.SessionTimelineResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/session/timeline", nil, &out); err != nil {
		return types.SessionTimelineResponse{}, err
	}
	return out, nil
}

func (c *Client) ListContextHistory(ctx context.Context) (types.ListContextHistoryResponse, error) {
	var out types.ListContextHistoryResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/session/history", nil, &out); err != nil {
		return types.ListContextHistoryResponse{}, err
	}
	return out, nil
}

func (c *Client) ReopenContext(ctx context.Context) (types.ContextHead, error) {
	var out types.ContextHead
	if err := c.doJSON(ctx, http.MethodPost, "/v1/session/reopen", map[string]any{}, &out); err != nil {
		return types.ContextHead{}, err
	}
	return out, nil
}

func (c *Client) LoadContextHistory(ctx context.Context, headID string) (types.ContextHead, error) {
	var out types.ContextHead
	if err := c.doJSON(ctx, http.MethodPost, "/v1/session/history/load", types.LoadContextHistoryRequest{
		HeadID: strings.TrimSpace(headID),
	}, &out); err != nil {
		return types.ContextHead{}, err
	}
	return out, nil
}

func (c *Client) GetWorkspaceMailbox(ctx context.Context) (types.WorkspaceReportMailboxResponse, error) {
	var out types.WorkspaceReportMailboxResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/mailbox", nil, &out); err != nil {
		return types.WorkspaceReportMailboxResponse{}, err
	}
	return out, nil
}

func (c *Client) GetRuntimeGraph(ctx context.Context) (types.WorkspaceRuntimeGraphResponse, error) {
	var out types.WorkspaceRuntimeGraphResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/runtime_graph", nil, &out); err != nil {
		return types.WorkspaceRuntimeGraphResponse{}, err
	}
	return out, nil
}

func (c *Client) GetReportingOverview(ctx context.Context, sessionID string) (types.ReportingOverview, error) {
	var out types.ReportingOverview
	path := "/v1/reporting/overview"
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		path += "?session_id=" + url.QueryEscape(trimmed)
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return types.ReportingOverview{}, err
	}
	return out, nil
}

func (c *Client) ListCronJobs(ctx context.Context, workspaceRoot string) (types.ListScheduledJobsResponse, error) {
	var out types.ListScheduledJobsResponse
	path := "/v1/cron"
	if trimmed := strings.TrimSpace(workspaceRoot); trimmed != "" {
		path += "?workspace_root=" + url.QueryEscape(trimmed)
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return types.ListScheduledJobsResponse{}, err
	}
	return out, nil
}

func (c *Client) GetCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	var out types.ScheduledJob
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/cron/%s", jobID), nil, &out); err != nil {
		return types.ScheduledJob{}, err
	}
	return out, nil
}

func (c *Client) PauseCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	var out types.ScheduledJob
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/cron/%s/pause", jobID), map[string]any{}, &out); err != nil {
		return types.ScheduledJob{}, err
	}
	return out, nil
}

func (c *Client) ResumeCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	var out types.ScheduledJob
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/cron/%s/resume", jobID), map[string]any{}, &out); err != nil {
		return types.ScheduledJob{}, err
	}
	return out, nil
}

func (c *Client) DeleteCronJob(ctx context.Context, jobID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/cron/%s", jobID), nil, nil)
}

func (c *Client) ListAutomations(ctx context.Context, workspaceRoot string) (types.ListAutomationsResponse, error) {
	var out types.ListAutomationsResponse
	path := "/v1/automations"
	q := url.Values{}
	if trimmed := strings.TrimSpace(workspaceRoot); trimmed != "" {
		q.Set("workspace_root", trimmed)
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return types.ListAutomationsResponse{}, err
	}
	return out, nil
}

func (c *Client) ApplyAutomation(ctx context.Context, req types.ApplyAutomationRequest) (types.AutomationSpec, error) {
	var out types.AutomationSpec
	if err := c.doJSON(ctx, http.MethodPost, "/v1/automations", req, &out); err != nil {
		return types.AutomationSpec{}, err
	}
	return out, nil
}

func (c *Client) GetAutomation(ctx context.Context, automationID string) (types.AutomationSpec, error) {
	var out types.AutomationSpec
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/automations/%s", automationID), nil, &out); err != nil {
		return types.AutomationSpec{}, err
	}
	return out, nil
}

func (c *Client) DeleteAutomation(ctx context.Context, automationID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/v1/automations/%s", automationID), nil, nil)
}

func (c *Client) PauseAutomation(ctx context.Context, automationID string) (types.AutomationSpec, error) {
	var out types.AutomationSpec
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/automations/%s/pause", automationID), map[string]any{}, &out); err != nil {
		return types.AutomationSpec{}, err
	}
	return out, nil
}

func (c *Client) ResumeAutomation(ctx context.Context, automationID string) (types.AutomationSpec, error) {
	var out types.AutomationSpec
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/automations/%s/resume", automationID), map[string]any{}, &out); err != nil {
		return types.AutomationSpec{}, err
	}
	return out, nil
}

func (c *Client) InstallAutomation(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, error) {
	var out types.AutomationWatcherRuntime
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/automations/%s/install", automationID), map[string]any{}, &out); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	return out, nil
}

func (c *Client) ReinstallAutomation(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, error) {
	var out types.AutomationWatcherRuntime
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/automations/%s/reinstall", automationID), map[string]any{}, &out); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	return out, nil
}

func (c *Client) GetAutomationWatcher(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, error) {
	var out types.AutomationWatcherRuntime
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/automations/%s/watcher", automationID), nil, &out); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	return out, nil
}

func (c *Client) EmitTrigger(ctx context.Context, req types.TriggerEmitRequest) (types.AutomationIncident, error) {
	var out types.AutomationIncident
	if err := c.doJSON(ctx, http.MethodPost, "/v1/triggers/emit", req, &out); err != nil {
		return types.AutomationIncident{}, err
	}
	return out, nil
}

func (c *Client) RecordHeartbeat(ctx context.Context, req types.TriggerHeartbeatRequest) (types.AutomationHeartbeat, error) {
	var out types.AutomationHeartbeat
	if err := c.doJSON(ctx, http.MethodPost, "/v1/triggers/heartbeat", req, &out); err != nil {
		return types.AutomationHeartbeat{}, err
	}
	return out, nil
}

func (c *Client) ListIncidents(ctx context.Context, filter types.IncidentListFilter) (types.ListAutomationIncidentsResponse, error) {
	var out types.ListAutomationIncidentsResponse
	path := "/v1/incidents"
	q := url.Values{}
	if trimmed := strings.TrimSpace(filter.WorkspaceRoot); trimmed != "" {
		q.Set("workspace_root", trimmed)
	}
	if trimmed := strings.TrimSpace(filter.AutomationID); trimmed != "" {
		q.Set("automation_id", trimmed)
	}
	if trimmed := strings.TrimSpace(string(filter.Status)); trimmed != "" {
		q.Set("status", trimmed)
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.Itoa(filter.Limit))
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return types.ListAutomationIncidentsResponse{}, err
	}
	return out, nil
}

func (c *Client) GetIncident(ctx context.Context, incidentID string) (types.AutomationIncident, error) {
	var out types.AutomationIncident
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/incidents/%s", incidentID), nil, &out); err != nil {
		return types.AutomationIncident{}, err
	}
	return out, nil
}

func (c *Client) ControlIncident(ctx context.Context, incidentID string, action types.IncidentControlAction) (types.AutomationIncident, error) {
	var out types.AutomationIncident
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/incidents/%s/%s", incidentID, action), map[string]any{}, &out); err != nil {
		return types.AutomationIncident{}, err
	}
	return out, nil
}

func (c *Client) ListPendingAutomationPermissions(ctx context.Context) (types.ListPendingAutomationPermissionsResponse, error) {
	var out types.ListPendingAutomationPermissionsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/permissions/pending", nil, &out); err != nil {
		return types.ListPendingAutomationPermissionsResponse{}, err
	}
	return out, nil
}

func (c *Client) GetPendingAutomationPermission(ctx context.Context, requestID string) (types.PendingAutomationPermission, error) {
	var out types.PendingAutomationPermission
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/permissions/pending/%s", requestID), nil, &out); err != nil {
		return types.PendingAutomationPermission{}, err
	}
	return out, nil
}

func (c *Client) StreamEvents(ctx context.Context, afterSeq int64) (<-chan types.Event, error) {
	params := url.Values{}
	params.Set("after", strconv.FormatInt(afterSeq, 10))
	params.Set("binding", sessionbinding.Normalize(c.contextBinding))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/session/events?%s", c.baseURL, params.Encode()), nil)
	if err != nil {
		return nil, err
	}
	c.applyContextBinding(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream events status: %d", resp.StatusCode)
	}

	out := make(chan types.Event, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var frame bytes.Buffer
		for {
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if event, ok := parseEventFrame(frame.String()); ok {
					out <- event
				}
				frame.Reset()
			} else {
				frame.WriteString(line)
				frame.WriteString("\n")
			}
			if errors.Is(err, io.EOF) {
				if event, ok := parseEventFrame(frame.String()); ok {
					out <- event
				}
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	var bodyReader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	c.applyContextBinding(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIError(resp, method, path)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) applyContextBinding(req *http.Request) {
	if c == nil || req == nil {
		return
	}
	req.Header.Set(sessionbinding.HeaderName, sessionbinding.Normalize(c.contextBinding))
	req.Header.Set(sessionrole.HeaderName, string(sessionrole.Normalize(string(c.sessionRole))))
}

func decodeAPIError(resp *http.Response, method string, path string) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}

	var validation types.AutomationValidationError
	if len(raw) > 0 && json.Unmarshal(raw, &validation) == nil && strings.TrimSpace(validation.Code) != "" {
		return &validation
	}

	if detail := strings.TrimSpace(string(raw)); detail != "" {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, detail)
	}

	return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
}

func parseEventFrame(frame string) (types.Event, bool) {
	lines := strings.Split(strings.TrimSpace(frame), "\n")
	if len(lines) == 0 {
		return types.Event{}, false
	}

	var eventName string
	dataLines := make([]string, 0, 1)
	var seq int64
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "id: "):
			parsed, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id: ")), 10, 64)
			if err == nil {
				seq = parsed
			}
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	data := strings.TrimSpace(strings.Join(dataLines, "\n"))
	if eventName == "" || eventName == "keepalive" || data == "" {
		return types.Event{}, false
	}

	var event types.Event
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return types.Event{}, false
	}
	if event.Seq == 0 {
		event.Seq = seq
	}
	return event, true
}
