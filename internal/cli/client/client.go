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

	"go-agent/internal/types"
)

type StatusResponse struct {
	Status               string `json:"status"`
	DaemonID             string `json:"daemon_id,omitempty"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	ConfigFingerprint    string `json:"config_fingerprint,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var out StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/status", nil, &out); err != nil {
		return StatusResponse{}, err
	}
	return out, nil
}

func (c *Client) ListSessions(ctx context.Context) (types.ListSessionsResponse, error) {
	var out types.ListSessionsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sessions", nil, &out); err != nil {
		return types.ListSessionsResponse{}, err
	}
	return out, nil
}

func (c *Client) CreateSession(ctx context.Context, req types.CreateSessionRequest) (types.Session, error) {
	var out types.Session
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sessions", req, &out); err != nil {
		return types.Session{}, err
	}
	return out, nil
}

func (c *Client) SelectSession(ctx context.Context, sessionID string) error {
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/sessions/%s/select", sessionID), map[string]any{}, nil)
}

func (c *Client) SubmitTurn(ctx context.Context, sessionID string, req types.SubmitTurnRequest) (types.Turn, error) {
	var out types.Turn
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/sessions/%s/turns", sessionID), req, &out); err != nil {
		return types.Turn{}, err
	}
	return out, nil
}

func (c *Client) InterruptTurn(ctx context.Context, sessionID string) error {
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/sessions/%s/interrupt", sessionID), nil, nil)
}

func (c *Client) DecidePermission(ctx context.Context, req types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error) {
	var out types.PermissionDecisionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/permissions/decide", req, &out); err != nil {
		return types.PermissionDecisionResponse{}, err
	}
	return out, nil
}

func (c *Client) GetTimeline(ctx context.Context, sessionID string) (types.SessionTimelineResponse, error) {
	var out types.SessionTimelineResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/sessions/%s/timeline", sessionID), nil, &out); err != nil {
		return types.SessionTimelineResponse{}, err
	}
	return out, nil
}

func (c *Client) GetReportMailbox(ctx context.Context, sessionID string) (types.SessionReportMailboxResponse, error) {
	var out types.SessionReportMailboxResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/sessions/%s/mailbox", sessionID), nil, &out); err != nil {
		return types.SessionReportMailboxResponse{}, err
	}
	return out, nil
}

func (c *Client) GetRuntimeGraph(ctx context.Context, sessionID string) (types.SessionRuntimeGraphResponse, error) {
	var out types.SessionRuntimeGraphResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/sessions/%s/runtime_graph", sessionID), nil, &out); err != nil {
		return types.SessionRuntimeGraphResponse{}, err
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

func (c *Client) FindOrCreateWorkspaceSession(ctx context.Context, workspaceRoot string) (string, bool, error) {
	resp, err := c.ListSessions(ctx)
	if err != nil {
		return "", false, err
	}
	for _, item := range resp.Sessions {
		if item.ID == resp.SelectedSessionID && item.WorkspaceRoot == workspaceRoot {
			return item.ID, false, nil
		}
	}
	for _, item := range resp.Sessions {
		if item.WorkspaceRoot == workspaceRoot {
			if err := c.SelectSession(ctx, item.ID); err != nil {
				return "", false, err
			}
			return item.ID, false, nil
		}
	}
	session, err := c.CreateSession(ctx, types.CreateSessionRequest{WorkspaceRoot: workspaceRoot})
	if err != nil {
		return "", false, err
	}
	if err := c.SelectSession(ctx, session.ID); err != nil {
		return "", false, err
	}
	return session.ID, true, nil
}

func (c *Client) StreamEvents(ctx context.Context, sessionID string, afterSeq int64) (<-chan types.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/sessions/%s/events?after=%d", c.baseURL, sessionID, afterSeq), nil)
	if err != nil {
		return nil, err
	}

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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
