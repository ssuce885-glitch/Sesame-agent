package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
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
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

const (
	defaultBaseURL       = "http://127.0.0.1:4317"
	defaultListenAddr    = "127.0.0.1:4317"
	defaultWorkspaceRoot = "/home/sauce/project/Workspace"
	sesameBinaryPath     = "/tmp/sesame"

	statusWaitTimeout = 30 * time.Second
	turnTimeout       = 120 * time.Second
)

type cliOptions struct {
	workspace string
	keep      bool
	turns     int
	verbose   bool
}

type statusPayload struct {
	Status               string `json:"status"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

type lifecycleTest struct {
	opts      cliOptions
	repoRoot  string
	workspace string
	dataDir   string
	dbPath    string

	httpClient *http.Client
	session    types.Session
	status     statusPayload
	daemon     *daemonProcess
	lastSeq    int64
}

type turnSpec struct {
	Title  string
	Prompt string
}

type daemonProcess struct {
	cmd        *exec.Cmd
	stderrFile *os.File
	stderrPath string
	done       chan error
}

type checkResult struct {
	Name   string
	Actual string
	Expect string
	Pass   bool
}

func main() {
	opts := parseFlags()
	if opts.turns <= 0 {
		fmt.Fprintln(os.Stderr, "-turns must be greater than 0")
		os.Exit(2)
	}

	test := &lifecycleTest{
		opts:       opts,
		httpClient: &http.Client{},
	}
	if err := test.run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() cliOptions {
	var opts cliOptions
	flag.StringVar(&opts.workspace, "workspace", "", "path to test workspace dir")
	flag.BoolVar(&opts.keep, "keep", false, "do not remove workspace after test")
	flag.IntVar(&opts.turns, "turns", 4, "number of turns to run")
	flag.BoolVar(&opts.verbose, "verbose", false, "print detailed output")
	flag.Parse()
	return opts
}

func (t *lifecycleTest) run(ctx context.Context) (err error) {
	t.repoRoot, err = findRepoRoot()
	if err != nil {
		return err
	}
	t.workspace, err = resolveWorkspace(t.opts.workspace)
	if err != nil {
		return err
	}
	t.dataDir = filepath.Join(t.workspace, ".sesame")
	t.dbPath = filepath.Join(t.dataDir, "sesame.db")

	fmt.Println("=== Sesame Lifecycle Test ===")
	fmt.Printf("Workspace: %s\n\n", t.workspace)

	cleanupStarted := false
	defer func() {
		if cleanupStarted {
			return
		}
		if cleanupErr := t.cleanup(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	if err := t.setup(ctx); err != nil {
		return err
	}

	fmt.Printf("Model: %s\n\n", formatModel(t.status))
	if err := t.ensureSession(ctx); err != nil {
		return err
	}

	turns := buildTurnSpecs(t.opts.turns)
	for idx, spec := range turns {
		if err := t.runTurn(ctx, idx+1, spec); err != nil {
			return err
		}
	}

	if err := t.verify(ctx, len(turns)); err != nil {
		return err
	}

	cleanupStarted = true
	return t.cleanup()
}

func (t *lifecycleTest) setup(ctx context.Context) error {
	fmt.Println("--- Setup ---")
	if err := os.MkdirAll(t.workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := workspace.Ensure(t.workspace, ""); err != nil {
		return fmt.Errorf("create workspace metadata: %w", err)
	}
	if err := os.MkdirAll(t.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	fmt.Print("Killing old daemon... ")
	if err := killExistingDaemon(ctx, defaultListenAddr, defaultBaseURL, t.httpClient); err != nil {
		fmt.Println("FAIL")
		return err
	}
	fmt.Println("OK")

	fmt.Print("Building daemon... ")
	if err := buildSesameDaemon(ctx, t.repoRoot, t.opts.verbose); err != nil {
		fmt.Println("FAIL")
		return err
	}
	fmt.Println("OK")

	fmt.Print("Starting daemon... ")
	daemon, err := startDaemon(t.workspace, t.dataDir)
	if err != nil {
		fmt.Println("FAIL")
		return err
	}
	t.daemon = daemon
	status, err := t.waitForStatus(ctx)
	if err != nil {
		fmt.Println("FAIL")
		return err
	}
	t.status = status
	fmt.Printf("OK (PID: %d)\n\n", status.PID)
	return nil
}

func buildSesameDaemon(ctx context.Context, repoRoot string, verbose bool) error {
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(buildCtx, "go", "build", "-o", sesameBinaryPath, "./cmd/sesame/")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if verbose && len(output) > 0 {
		fmt.Print(string(output))
	}
	if err != nil {
		return fmt.Errorf("go build cmd/sesame: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func startDaemon(workspaceRoot, dataDir string) (*daemonProcess, error) {
	stderrPath := filepath.Join(dataDir, "daemon.stderr.log")
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(sesameBinaryPath, "daemon")
	cmd.Dir = workspaceRoot
	cmd.Stdout = io.Discard
	cmd.Stderr = stderrFile
	cmd.Env = envWithOverrides(os.Environ(), map[string]string{
		"SESAME_ADDR":     defaultListenAddr,
		"SESAME_DATA_DIR": dataDir,
	})
	if err := cmd.Start(); err != nil {
		_ = stderrFile.Close()
		return nil, err
	}

	proc := &daemonProcess{
		cmd:        cmd,
		stderrFile: stderrFile,
		stderrPath: stderrPath,
		done:       make(chan error, 1),
	}
	go func() {
		err := cmd.Wait()
		_ = stderrFile.Close()
		proc.done <- err
	}()
	return proc, nil
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

func (t *lifecycleTest) waitForStatus(ctx context.Context) (statusPayload, error) {
	deadline := time.Now().Add(statusWaitTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if exitErr, exited := t.daemon.exited(); exited {
			return statusPayload{}, fmt.Errorf("daemon exited before status was ready: %v\n%s", exitErr, tailFile(t.daemon.stderrPath, 8192))
		}

		status, err := fetchStatus(ctx, defaultBaseURL, t.httpClient)
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
	return statusPayload{}, fmt.Errorf("timed out waiting for /v1/status: %w\n%s", lastErr, tailFile(t.daemon.stderrPath, 8192))
}

func (t *lifecycleTest) ensureSession(ctx context.Context) error {
	var session types.Session
	if err := t.doJSON(ctx, http.MethodPost, "/v1/session/ensure", types.EnsureSessionRequest{
		WorkspaceRoot: t.workspace,
	}, &session); err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}
	t.session = session
	if t.opts.verbose {
		fmt.Printf("Session: %s\n\n", session.ID)
	}
	return nil
}

func (t *lifecycleTest) runTurn(ctx context.Context, number int, spec turnSpec) error {
	fmt.Printf("--- Turn %d: %s ---\n", number, spec.Title)
	fmt.Printf("Prompt: %s\n", preview(spec.Prompt, 88))

	start := time.Now()
	turn, err := t.submitTurn(ctx, number, spec.Prompt)
	if err != nil {
		return err
	}

	printed := false
	endsWithNewline := true
	err = t.streamUntilTurnComplete(ctx, turn.ID, func(text string) {
		if text == "" {
			return
		}
		printed = true
		endsWithNewline = strings.HasSuffix(text, "\n")
		fmt.Print(text)
	})
	if err != nil {
		return err
	}
	if printed && !endsWithNewline {
		fmt.Println()
	}
	fmt.Printf("OK (%.1fs)\n\n", time.Since(start).Seconds())
	return nil
}

func (t *lifecycleTest) submitTurn(ctx context.Context, number int, prompt string) (types.Turn, error) {
	var turn types.Turn
	req := types.SubmitTurnRequest{
		ClientTurnID: fmt.Sprintf("lifecycle-%d-%d", number, time.Now().UTC().UnixNano()),
		Message:      prompt,
	}
	if err := t.doJSON(ctx, http.MethodPost, "/v1/session/turns", req, &turn); err != nil {
		return types.Turn{}, fmt.Errorf("submit turn %d: %w", number, err)
	}
	if t.opts.verbose {
		fmt.Printf("[turn_id=%s]\n", turn.ID)
	}
	return turn, nil
}

func (t *lifecycleTest) streamUntilTurnComplete(ctx context.Context, turnID string, onDelta func(string)) error {
	turnCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		completed, err := t.streamTurnOnce(turnCtx, turnID, onDelta)
		if completed {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = io.ErrUnexpectedEOF
		}
		if attempt == 0 {
			if t.opts.verbose {
				fmt.Printf("\n[SSE disconnected, retrying once after seq %d: %v]\n", t.lastSeq, lastErr)
			}
			continue
		}
	}
	return fmt.Errorf("stream turn %s: %w", turnID, lastErr)
}

func (t *lifecycleTest) streamTurnOnce(ctx context.Context, turnID string, onDelta func(string)) (bool, error) {
	params := url.Values{}
	params.Set("after", strconv.FormatInt(t.lastSeq, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultBaseURL+"/v1/session/events?"+params.Encode(), nil)
	if err != nil {
		return false, err
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("events status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var frame sseFrame
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			completed, err := t.handleSSEFrame(frame, turnID, onDelta)
			frame = sseFrame{}
			if completed || err != nil {
				return completed, err
			}
			continue
		}
		frame.addLine(line)
	}
	if len(frame.dataLines) > 0 {
		completed, err := t.handleSSEFrame(frame, turnID, onDelta)
		if completed || err != nil {
			return completed, err
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, io.ErrUnexpectedEOF
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
		if strings.HasPrefix(data, " ") {
			data = data[1:]
		}
		f.dataLines = append(f.dataLines, data)
	case strings.HasPrefix(line, "id:"):
		return
	default:
		return
	}
}

func (t *lifecycleTest) handleSSEFrame(frame sseFrame, turnID string, onDelta func(string)) (bool, error) {
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
	if event.Seq > t.lastSeq {
		t.lastSeq = event.Seq
	}
	if event.TurnID != "" && event.TurnID != turnID {
		if t.opts.verbose {
			fmt.Printf("\n[event %s for other turn %s]\n", event.Type, event.TurnID)
		}
		return false, nil
	}

	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return false, err
		}
		onDelta(payload.Text)
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
	default:
		if t.opts.verbose && strings.HasPrefix(event.Type, "tool.") {
			fmt.Printf("\n[%s]\n", event.Type)
		}
	}
	return false, nil
}

func (t *lifecycleTest) verify(ctx context.Context, expectedTurns int) error {
	fmt.Println("--- Verification ---")

	store, err := sqlite.Open(t.dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite store %s: %w", t.dbPath, err)
	}
	defer store.Close()

	if err := waitForSummary(ctx, store, t.session.ID); err != nil && t.opts.verbose {
		fmt.Printf("Summary wait: %v\n", err)
	}

	entries, err := store.ListVisibleMemoryEntries(ctx, t.workspace, "")
	if err != nil {
		return fmt.Errorf("list visible memory entries: %w", err)
	}
	if _, _, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: t.workspace,
		TextQuery:   "database deployment",
		Limit:       5,
	}); err != nil {
		return fmt.Errorf("search cold index: %w", err)
	}

	summaryCount, err := queryCount(ctx, store, `select count(*) from context_head_summaries where session_id = ?`, t.session.ID)
	if err != nil {
		return err
	}
	conversationCount, err := queryCount(ctx, store, `select count(*) from conversation_items where session_id = ?`, t.session.ID)
	if err != nil {
		return err
	}
	turnCount, err := queryCount(ctx, store, `select count(*) from turns where session_id = ?`, t.session.ID)
	if err != nil {
		return err
	}

	kindCounts := map[types.MemoryKind]int{}
	for _, entry := range entries {
		kindCounts[entry.Kind]++
	}
	coreKindCount := kindCounts[types.MemoryKindFact] + kindCounts[types.MemoryKindPreference] + kindCounts[types.MemoryKindDecision]

	checks := []checkResult{
		{Name: "Memory entries", Actual: strconv.Itoa(len(entries)), Expect: ">= 5", Pass: len(entries) >= 5},
		{Name: "Context head summaries", Actual: strconv.Itoa(summaryCount), Expect: ">= 1", Pass: summaryCount >= 1},
		{Name: "Conversation items", Actual: strconv.Itoa(conversationCount), Expect: ">= 20", Pass: conversationCount >= 20},
		{Name: "Turns", Actual: strconv.Itoa(turnCount), Expect: fmt.Sprintf(">= %d", expectedTurns), Pass: turnCount >= expectedTurns},
		{
			Name: "Memory kinds",
			Actual: fmt.Sprintf(
				"fact=%d preference=%d decision=%d",
				kindCounts[types.MemoryKindFact],
				kindCounts[types.MemoryKindPreference],
				kindCounts[types.MemoryKindDecision],
			),
			Expect: "at least one",
			Pass:   coreKindCount > 0,
		},
	}

	passing := 0
	var failed []string
	for _, check := range checks {
		status := "FAIL"
		if check.Pass {
			status = "PASS"
			passing++
		} else {
			failed = append(failed, check.Name)
		}
		fmt.Printf("%s: %s (expect %s) %s\n", check.Name, check.Actual, check.Expect, status)
	}
	fmt.Printf("\nTotal: %d/%d passing\n", passing, len(checks))

	if len(failed) > 0 {
		fmt.Printf("FAIL: %d memories, %d summaries, %d conversation items\n\n", len(entries), summaryCount, conversationCount)
		return fmt.Errorf("verification failed for %s; inspect %s and daemon stderr at %s", strings.Join(failed, ", "), t.dbPath, daemonLogPath(t.daemon))
	}
	fmt.Printf("PASS: %d memories, %d summaries, %d conversation items\n\n", len(entries), summaryCount, conversationCount)
	return nil
}

func waitForSummary(ctx context.Context, store *sqlite.Store, sessionID string) error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		count, err := queryCount(ctx, store, `select count(*) from context_head_summaries where session_id = ?`, sessionID)
		if err != nil {
			return err
		}
		if count >= 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for context_head_summaries")
		}
		time.Sleep(1 * time.Second)
	}
}

func queryCount(ctx context.Context, store *sqlite.Store, query string, args ...any) (int, error) {
	var count int
	if err := store.DB().QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (t *lifecycleTest) cleanup() error {
	fmt.Println("--- Cleanup ---")
	fmt.Print("Killing daemon... ")
	if t.daemon != nil {
		if err := t.daemon.stop(); err != nil {
			fmt.Println("FAIL")
			return err
		}
	}
	fmt.Println("OK")

	if t.opts.keep {
		fmt.Printf("Keeping workspace: %s\n", t.workspace)
		return nil
	}

	fmt.Print("Removing workspace... ")
	if err := os.RemoveAll(t.workspace); err != nil {
		fmt.Println("FAIL")
		return err
	}
	fmt.Println("OK")
	return nil
}

func killExistingDaemon(ctx context.Context, addr, baseURL string, httpClient *http.Client) error {
	pids := map[int]struct{}{}
	if status, err := fetchStatus(ctx, baseURL, httpClient); err == nil && status.PID > 0 {
		pids[status.PID] = struct{}{}
	}
	for _, pid := range pidsListeningOnPort(addr) {
		pids[pid] = struct{}{}
	}

	for pid := range pids {
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		if err := killPID(pid); err != nil {
			return fmt.Errorf("kill pid %d: %w", pid, err)
		}
	}

	if len(pids) == 0 && isTCPPortOpen(addr) {
		return fmt.Errorf("port %s is in use, but no daemon pid could be resolved", addr)
	}
	return waitForPortClosed(addr, 5*time.Second)
}

func fetchStatus(ctx context.Context, baseURL string, httpClient *http.Client) (statusPayload, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/status", nil)
	if err != nil {
		return statusPayload{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return statusPayload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusPayload{}, fmt.Errorf("status code %d", resp.StatusCode)
	}
	var payload statusPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return statusPayload{}, err
	}
	return payload, nil
}

func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil {
		return err
	}
	_ = proc.Release()
	return nil
}

func pidsListeningOnPort(addr string) []int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}
	var out []int
	if path, err := exec.LookPath("lsof"); err == nil {
		cmd := exec.Command(path, "-ti", "tcp:"+port, "-sTCP:LISTEN")
		if raw, err := cmd.Output(); err == nil {
			out = append(out, parsePIDs(string(raw))...)
		}
	}
	if len(out) > 0 {
		return uniqueInts(out)
	}
	if path, err := exec.LookPath("fuser"); err == nil {
		cmd := exec.Command(path, port+"/tcp")
		if raw, err := cmd.CombinedOutput(); err == nil {
			out = append(out, parsePIDs(string(raw))...)
		}
	}
	return uniqueInts(out)
}

func parsePIDs(raw string) []int {
	fields := strings.Fields(raw)
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if strings.HasSuffix(field, ":") {
			continue
		}
		pid, err := strconv.Atoi(field)
		if err == nil && pid > 0 {
			out = append(out, pid)
		}
	}
	return out
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isTCPPortOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForPortClosed(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !isTCPPortOpen(addr) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for port %s to close", addr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (t *lifecycleTest) doJSON(ctx context.Context, method, path string, body any, out any) error {
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
	req, err := http.NewRequestWithContext(reqCtx, method, defaultBaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.httpClient.Do(req)
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

func resolveWorkspace(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return filepath.Abs(trimmed)
	}
	if err := os.MkdirAll(defaultWorkspaceRoot, 0o755); err != nil {
		return "", err
	}
	for attempts := 0; attempts < 100; attempts++ {
		path := filepath.Join(defaultWorkspaceRoot, fmt.Sprintf("sesame-test-%04d", randomFourDigits()))
		if err := os.Mkdir(path, 0o755); err == nil {
			return path, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not create unique workspace under %s", defaultWorkspaceRoot)
}

func randomFourDigits() int {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return int(binary.BigEndian.Uint64(buf[:]) % 10000)
	}
	return int(time.Now().UnixNano() % 10000)
}

func findRepoRoot() (string, error) {
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func buildTurnSpecs(count int) []turnSpec {
	base := []turnSpec{
		{
			Title:  "Knowledge seeding",
			Prompt: "Please record the following facts about this project using memory_write: 1) We use PostgreSQL for primary database, 2) Our deployment target is Kubernetes on AWS, 3) All API endpoints must have rate limiting.",
		},
		{
			Title:  "Continue context",
			Prompt: "Also record that: we run integration tests before every deploy, and we use Prometheus + Grafana for monitoring.",
		},
		{
			Title:  "Search past work",
			Prompt: "Use recall_archive to search for any past discussions about database or deployment topics. Report what you find.",
		},
		{
			Title:  "Reflect",
			Prompt: "Based on everything we've discussed, what conventions and patterns have we established for this project?",
		},
	}
	if count <= len(base) {
		return append([]turnSpec(nil), base[:count]...)
	}
	out := append([]turnSpec(nil), base...)
	for idx := len(base) + 1; idx <= count; idx++ {
		out = append(out, turnSpec{
			Title:  fmt.Sprintf("Extra lifecycle check %d", idx),
			Prompt: "Continue the lifecycle test by briefly confirming the project conventions remembered so far.",
		})
	}
	return out
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

func formatModel(status statusPayload) string {
	model := strings.TrimSpace(status.Model)
	provider := strings.TrimSpace(status.Provider)
	switch {
	case provider != "" && model != "":
		return provider + "/" + model
	case model != "":
		return model
	case provider != "":
		return provider
	default:
		return "(unknown)"
	}
}

func preview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "..."
}

func daemonLogPath(p *daemonProcess) string {
	if p == nil || strings.TrimSpace(p.stderrPath) == "" {
		return "(none)"
	}
	return p.stderrPath
}

func tailFile(path string, limit int64) string {
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
