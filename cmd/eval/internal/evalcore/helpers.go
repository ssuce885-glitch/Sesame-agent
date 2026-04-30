package evalcore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

const (
	statusWaitTimeout = 30 * time.Second
	turnTimeout       = 180 * time.Second
)

var (
	buildDaemonOnce sync.Once
	buildDaemonPath string
	buildDaemonErr  error

	daemonMu    sync.Mutex
	daemonProcs = map[int]*daemonProcess{}
)

type StatusPayload struct {
	Status               string `json:"status"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

type daemonProcess struct {
	cmd        *exec.Cmd
	addr       string
	baseURL    string
	stderrFile *os.File
	stderrPath string
	done       chan error
}

func StartDaemon(workspaceRoot string) (int, string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return 0, "", fmt.Errorf("workspace root is required")
	}
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(absWorkspace, 0o755); err != nil {
		return 0, "", fmt.Errorf("create workspace: %w", err)
	}
	if _, err := workspace.Ensure(absWorkspace, ""); err != nil {
		return 0, "", fmt.Errorf("create workspace metadata: %w", err)
	}
	dataDir := filepath.Join(absWorkspace, ".sesame")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("create data dir: %w", err)
	}

	binaryPath, err := daemonBinary()
	if err != nil {
		return 0, "", err
	}
	addr, err := reserveTCPAddr()
	if err != nil {
		return 0, "", err
	}
	baseURL := "http://" + addr
	stderrPath := filepath.Join(dataDir, "eval-daemon.stderr.log")
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return 0, "", err
	}

	cmd := exec.Command(binaryPath, "daemon")
	cmd.Dir = absWorkspace
	cmd.Stdout = io.Discard
	cmd.Stderr = stderrFile
	cmd.Env = envWithOverrides(os.Environ(), map[string]string{
		"SESAME_ADDR":     addr,
		"SESAME_DATA_DIR": dataDir,
	})
	if err := cmd.Start(); err != nil {
		_ = stderrFile.Close()
		return 0, "", err
	}

	proc := &daemonProcess{
		cmd:        cmd,
		addr:       addr,
		baseURL:    baseURL,
		stderrFile: stderrFile,
		stderrPath: stderrPath,
		done:       make(chan error, 1),
	}
	go func() {
		err := cmd.Wait()
		_ = stderrFile.Close()
		proc.done <- err
	}()

	daemonMu.Lock()
	daemonProcs[cmd.Process.Pid] = proc
	daemonMu.Unlock()

	if _, err := WaitForStatus(context.Background(), baseURL, http.DefaultClient, proc); err != nil {
		_ = StopDaemon(cmd.Process.Pid)
		return 0, "", err
	}
	return cmd.Process.Pid, baseURL, nil
}

func StopDaemon(pid int) error {
	if pid <= 0 {
		return nil
	}

	daemonMu.Lock()
	proc := daemonProcs[pid]
	delete(daemonProcs, pid)
	daemonMu.Unlock()

	if proc != nil {
		return proc.stop()
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil {
		return err
	}
	_ = process.Release()
	return nil
}

func EnsureSessionContext(ctx context.Context, baseURL, workspaceRoot string) (string, error) {
	var session types.Session
	err := DoJSON(ctx, http.DefaultClient, baseURL, http.MethodPost, "/v1/session/ensure", types.EnsureSessionRequest{
		WorkspaceRoot: workspaceRoot,
	}, &session)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(session.ID) == "" {
		return "", fmt.Errorf("ensure session returned empty session id")
	}
	return session.ID, nil
}

func SendTurnContext(ctx context.Context, baseURL, sessionID, message string) (EvalResponse, error) {
	turn, err := SubmitTurn(ctx, baseURL, message)
	if err != nil {
		return EvalResponse{}, err
	}
	return StreamTurnUntilComplete(ctx, baseURL, sessionID, turn.ID)
}

func SubmitTurn(ctx context.Context, baseURL, message string) (types.Turn, error) {
	var turn types.Turn
	req := types.SubmitTurnRequest{
		ClientTurnID: fmt.Sprintf("eval-%d-%d", time.Now().UTC().UnixNano(), randomFourDigits()),
		Message:      message,
	}
	if err := DoJSON(ctx, http.DefaultClient, baseURL, http.MethodPost, "/v1/session/turns", req, &turn); err != nil {
		return types.Turn{}, err
	}
	if strings.TrimSpace(turn.ID) == "" {
		return types.Turn{}, fmt.Errorf("submit turn returned empty turn id")
	}
	return turn, nil
}

func StreamTurnUntilComplete(ctx context.Context, baseURL, sessionID, turnID string) (EvalResponse, error) {
	if strings.TrimSpace(sessionID) == "" {
		return EvalResponse{}, fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(turnID) == "" {
		return EvalResponse{}, fmt.Errorf("turn id is required")
	}

	streamCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()

	response := EvalResponse{TurnID: turnID}
	seenTools := map[string]struct{}{}
	params := url.Values{}
	params.Set("after", "0")
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/session/events?"+params.Encode(), nil)
	if err != nil {
		return response, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return response, fmt.Errorf("events status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var frame sseFrame
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			completed, err := handleTurnFrame(frame, turnID, &response, seenTools)
			frame = sseFrame{}
			if completed || err != nil {
				return response, err
			}
			continue
		}
		frame.addLine(line)
	}
	if len(frame.dataLines) > 0 {
		completed, err := handleTurnFrame(frame, turnID, &response, seenTools)
		if completed || err != nil {
			return response, err
		}
	}
	if err := scanner.Err(); err != nil {
		return response, err
	}
	return response, io.ErrUnexpectedEOF
}

func DoJSON(ctx context.Context, client *http.Client, baseURL, method, path string, body any, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func FetchStatus(ctx context.Context, baseURL string, client *http.Client) (StatusPayload, error) {
	if client == nil {
		client = http.DefaultClient
	}
	var payload StatusPayload
	if err := DoJSON(ctx, client, baseURL, http.MethodGet, "/v1/status", nil, &payload); err != nil {
		return StatusPayload{}, err
	}
	return payload, nil
}

func WaitForStatus(ctx context.Context, baseURL string, client *http.Client, proc *daemonProcess) (StatusPayload, error) {
	deadline := time.Now().Add(statusWaitTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if proc != nil {
			if exitErr, exited := proc.exited(); exited {
				return StatusPayload{}, fmt.Errorf("daemon exited before status was ready: %v\n%s", exitErr, TailFile(proc.stderrPath, 8192))
			}
		}
		status, err := FetchStatus(ctx, baseURL, client)
		if err == nil && strings.TrimSpace(status.Status) == "ok" {
			return status, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("status was %q", status.Status)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if proc != nil {
		return StatusPayload{}, fmt.Errorf("timed out waiting for /v1/status: %w\n%s", lastErr, TailFile(proc.stderrPath, 8192))
	}
	return StatusPayload{}, fmt.Errorf("timed out waiting for /v1/status: %w", lastErr)
}

func WaitForTurnTerminal(ctx context.Context, dbPath, turnID string, timeout time.Duration) (types.TurnState, error) {
	if timeout <= 0 {
		timeout = turnTimeout
	}
	store, err := sqlite.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer store.Close()

	deadline := time.Now().Add(timeout)
	for {
		turn, found, err := store.GetTurn(ctx, turnID)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("turn %s not found", turnID)
		}
		if IsTerminalTurnState(turn.State) {
			return turn.State, nil
		}
		if time.Now().After(deadline) {
			return turn.State, fmt.Errorf("timed out waiting for turn %s terminal state; latest state %s", turnID, turn.State)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func WaitForTurnStateChange(ctx context.Context, dbPath, turnID string, initial types.TurnState, timeout time.Duration) (types.TurnState, error) {
	store, err := sqlite.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer store.Close()
	deadline := time.Now().Add(timeout)
	for {
		turn, found, err := store.GetTurn(ctx, turnID)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("turn %s not found", turnID)
		}
		if turn.State != initial {
			return turn.State, nil
		}
		if time.Now().After(deadline) {
			return turn.State, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func IsTerminalTurnState(state types.TurnState) bool {
	switch state {
	case types.TurnStateCompleted, types.TurnStateFailed, types.TurnStateInterrupted:
		return true
	default:
		return false
	}
}

func SQLiteIntegrityOK(ctx context.Context, dbPath string) EvalResult {
	store, err := sqlite.Open(dbPath)
	if err != nil {
		return Result("sqlite integrity", false, err.Error())
	}
	defer store.Close()

	var status string
	if err := store.DB().QueryRowContext(ctx, `pragma integrity_check`).Scan(&status); err != nil {
		return Result("sqlite integrity", false, err.Error())
	}
	return Result("sqlite integrity", status == "ok", status)
}

func TailFile(path string, limit int64) string {
	if strings.TrimSpace(path) == "" || limit <= 0 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := info.Size() - limit
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "(daemon stderr was empty)"
	}
	return text
}

func (p *daemonProcess) stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	select {
	case <-p.done:
		return nil
	default:
	}

	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for daemon process to exit")
	}
}

func (p *daemonProcess) exited() (error, bool) {
	if p == nil {
		return nil, false
	}
	select {
	case err := <-p.done:
		p.done <- err
		return err, true
	default:
		return nil, false
	}
}

type sseFrame struct {
	eventName string
	dataLines []string
}

func (f *sseFrame) addLine(line string) {
	switch {
	case strings.HasPrefix(line, "event:"):
		f.eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
	case strings.HasPrefix(line, "data:"):
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		f.dataLines = append(f.dataLines, data)
	case strings.HasPrefix(line, "id:"):
		return
	default:
		return
	}
}

func handleTurnFrame(frame sseFrame, turnID string, response *EvalResponse, seenTools map[string]struct{}) (bool, error) {
	if frame.eventName == "" || frame.eventName == "keepalive" || len(frame.dataLines) == 0 {
		return false, nil
	}
	var event types.Event
	if err := json.Unmarshal([]byte(strings.Join(frame.dataLines, "\n")), &event); err != nil {
		return false, err
	}
	if event.Type == "" {
		event.Type = frame.eventName
	}
	if event.TurnID != "" && event.TurnID != turnID {
		return false, nil
	}

	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return false, err
		}
		response.AssistantText += payload.Text
	case types.EventToolStarted, types.EventToolCompleted:
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			name := strings.TrimSpace(payload.ToolName)
			if name != "" {
				key := event.Type + ":" + name + ":" + payload.ToolCallID
				if _, ok := seenTools[key]; !ok {
					seenTools[key] = struct{}{}
					response.ToolCalls = append(response.ToolCalls, name)
				}
			}
		}
	case types.EventTurnCompleted:
		return true, nil
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		_ = json.Unmarshal(event.Payload, &payload)
		if strings.TrimSpace(payload.Message) == "" {
			payload.Message = "turn failed"
		}
		return false, errors.New(payload.Message)
	case types.EventTurnInterrupted:
		return false, errors.New("turn interrupted")
	}
	return false, nil
}

func daemonBinary() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("SESAME_EVAL_DAEMON_BIN")); explicit != "" {
		if info, err := os.Stat(explicit); err != nil {
			return "", err
		} else if info.IsDir() {
			return "", fmt.Errorf("SESAME_EVAL_DAEMON_BIN points to a directory: %s", explicit)
		}
		return explicit, nil
	}
	buildDaemonOnce.Do(func() {
		repoRoot, err := FindRepoRoot()
		if err != nil {
			buildDaemonErr = err
			return
		}
		buildDaemonPath = filepath.Join(os.TempDir(), "sesame-eval-daemon")
		buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(buildCtx, "go", "build", "-o", buildDaemonPath, "./cmd/sesame/")
		cmd.Dir = repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			buildDaemonErr = fmt.Errorf("go build cmd/sesame: %w\n%s", err, strings.TrimSpace(string(output)))
		}
	})
	if buildDaemonErr != nil {
		return "", buildDaemonErr
	}
	return buildDaemonPath, nil
}

func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for current := cwd; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "go.mod")) && fileExists(filepath.Join(current, "cmd", "sesame", "main.go")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("could not find repository root from %s", cwd)
}

func reserveTCPAddr() (string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected listener address %T", listener.Addr())
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(addr.Port)), nil
}

func envWithOverrides(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		seen[key] = struct{}{}
	}
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := seen[key]; overridden {
				continue
			}
		}
		out = append(out, entry)
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func randomFourDigits() int {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return int(binary.BigEndian.Uint64(buf[:]) % 10000)
	}
	return int(time.Now().UnixNano() % 10000)
}
