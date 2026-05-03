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
	"time"

	"go-agent/internal/v2/tui"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

func (c *Client) Status(ctx context.Context) (tui.StatusResponse, error) {
	var out tui.StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/status", nil, &out); err != nil {
		return tui.StatusResponse{}, err
	}
	return out, nil
}

func (c *Client) SubmitTurn(ctx context.Context, req tui.SubmitTurnRequest) (tui.Turn, error) {
	var out tui.Turn
	if err := c.doJSON(ctx, http.MethodPost, "/v2/turns", req, &out); err != nil {
		return tui.Turn{}, err
	}
	return out, nil
}

func (c *Client) InterruptTurn(ctx context.Context, sessionID string) error {
	session, err := c.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(session.ActiveTurnID) == "" {
		return fmt.Errorf("no running turn")
	}
	return c.doJSON(ctx, http.MethodPost, "/v2/turns/"+url.PathEscape(session.ActiveTurnID)+"/interrupt", nil, nil)
}

func (c *Client) StreamEvents(ctx context.Context, sessionID string, afterSeq int64) (<-chan tui.Event, <-chan error, error) {
	params := url.Values{}
	params.Set("session_id", sessionID)
	params.Set("after", strconv.FormatInt(afterSeq, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v2/events?"+params.Encode(), nil)
	if err != nil {
		return nil, nil, err
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("stream events status: %d", resp.StatusCode)
	}

	out := make(chan tui.Event, 32)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var frame bytes.Buffer
		for {
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				errs <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if event, ok := parseEventFrame(frame.String(), sessionID); ok {
					select {
					case out <- event:
					case <-ctx.Done():
						return
					}
				}
				frame.Reset()
			} else {
				frame.WriteString(line)
				frame.WriteByte('\n')
			}
			if errors.Is(err, io.EOF) {
				if event, ok := parseEventFrame(frame.String(), sessionID); ok {
					select {
					case out <- event:
					case <-ctx.Done():
						return
					}
				}
				errs <- nil
				return
			}
		}
	}()
	return out, errs, nil
}

func (c *Client) GetTimeline(ctx context.Context, sessionID string) (tui.SessionTimelineResponse, error) {
	var out tui.SessionTimelineResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/sessions/"+url.PathEscape(sessionID)+"/timeline", nil, &out); err != nil {
		return tui.SessionTimelineResponse{}, err
	}
	return out, nil
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (tui.SessionInfo, error) {
	var out tui.SessionInfo
	if err := c.doJSON(ctx, http.MethodGet, "/v2/sessions/"+url.PathEscape(sessionID), nil, &out); err != nil {
		return tui.SessionInfo{}, err
	}
	return out, nil
}

func (c *Client) GetWorkspaceReports(ctx context.Context, workspaceRoot string) (tui.ReportsResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(workspaceRoot) != "" {
		params.Set("workspace_root", workspaceRoot)
	}
	path := "/v2/reports"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out tui.ReportsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return tui.ReportsResponse{}, err
	}
	return out, nil
}

func (c *Client) GetAutomations(ctx context.Context, workspaceRoot string) ([]tui.AutomationResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(workspaceRoot) != "" {
		params.Set("workspace_root", workspaceRoot)
	}
	path := "/v2/automations"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out []tui.AutomationResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetProjectState(ctx context.Context, workspaceRoot string) (tui.ProjectStateResponse, error) {
	params := url.Values{}
	if strings.TrimSpace(workspaceRoot) != "" {
		params.Set("workspace_root", workspaceRoot)
	}
	path := "/v2/project_state"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out tui.ProjectStateResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return tui.ProjectStateResponse{}, err
	}
	return out, nil
}

func (c *Client) UpdateProjectState(ctx context.Context, workspaceRoot, summary string) (tui.ProjectStateResponse, error) {
	var out tui.ProjectStateResponse
	body := map[string]string{
		"workspace_root": workspaceRoot,
		"summary":        summary,
	}
	if err := c.doJSON(ctx, http.MethodPut, "/v2/project_state", body, &out); err != nil {
		return tui.ProjectStateResponse{}, err
	}
	return out, nil
}

func (c *Client) GetSetting(ctx context.Context, key string) (tui.SettingResponse, error) {
	var out tui.SettingResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/settings/"+url.PathEscape(key), nil, &out); err != nil {
		return tui.SettingResponse{}, err
	}
	return out, nil
}

func (c *Client) SetSetting(ctx context.Context, key, value string) (tui.SettingResponse, error) {
	var out tui.SettingResponse
	if err := c.doJSON(ctx, http.MethodPut, "/v2/settings/"+url.PathEscape(key), map[string]string{"value": value}, &out); err != nil {
		return tui.SettingResponse{}, err
	}
	return out, nil
}

func (c *Client) EnsureSession(ctx context.Context, workspaceRoot string) (tui.SessionInfo, error) {
	var out tui.SessionInfo
	if err := c.doJSON(ctx, http.MethodPost, "/v2/sessions", map[string]string{"workspace_root": workspaceRoot}, &out); err != nil {
		return tui.SessionInfo{}, err
	}
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
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

func parseEventFrame(frame, sessionID string) (tui.Event, bool) {
	lines := strings.Split(strings.TrimSpace(frame), "\n")
	if len(lines) == 0 {
		return tui.Event{}, false
	}

	var typ string
	var seq int64
	dataLines := make([]string, 0, 1)
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "id:"):
			parsed, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id:")), 10, 64)
			if err == nil {
				seq = parsed
			}
		case strings.HasPrefix(line, "event:"):
			typ = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if typ == "" || typ == "keepalive" || len(dataLines) == 0 {
		return tui.Event{}, false
	}
	return tui.Event{
		Seq:       seq,
		SessionID: sessionID,
		Type:      typ,
		Payload:   []byte(strings.Join(dataLines, "\n")),
		Time:      time.Now(),
	}, true
}

func decodeAPIError(resp *http.Response, method, path string) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &body) == nil && strings.TrimSpace(body.Error) != "" {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, body.Error)
	}
	if detail := strings.TrimSpace(string(raw)); detail != "" {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, detail)
	}
	return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
}
