package evalcore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Logger func(format string, args ...any)

var unsafePathChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func RunSuites(ctx context.Context, suites []EvalSuite, opts RunnerOptions, logf Logger) (RunReport, error) {
	startedClock := time.Now()
	started := startedClock.UTC()
	if logf == nil {
		logf = func(string, ...any) {}
	}
	selected, err := filterSuites(suites, opts.SuiteFilter)
	if err != nil {
		return RunReport{}, err
	}

	baseWorkspace, removeBase, err := resolveEvalWorkspaceRoot(opts.WorkspaceRoot)
	if err != nil {
		return RunReport{}, err
	}
	if removeBase && !opts.Keep {
		defer func() { _ = os.RemoveAll(baseWorkspace) }()
	}

	report := RunReport{
		StartedAt: started,
		Passed:    true,
		Suites:    make([]SuiteReport, 0, len(selected)),
	}
	for _, suite := range selected {
		suiteReport := runSuite(ctx, suite, baseWorkspace, opts, logf)
		report.Suites = append(report.Suites, suiteReport)
		if !suiteReport.Passed {
			report.Passed = false
		}
	}
	duration := time.Since(startedClock)
	if duration < 0 {
		duration = 0
	}
	report.DurationMS = duration.Milliseconds()
	if !report.Passed {
		return report, errors.New("one or more eval suites failed")
	}
	return report, nil
}

func runSuite(ctx context.Context, suite EvalSuite, baseWorkspace string, opts RunnerOptions, logf Logger) (report SuiteReport) {
	minPassRate := suite.MinPassRate
	workspaceRoot := filepath.Join(baseWorkspace, fmt.Sprintf("%s-%d", safeSuiteName(suite.Name), time.Now().UTC().UnixNano()))
	report = SuiteReport{
		Name:        suite.Name,
		Description: suite.Description,
		Workspace:   workspaceRoot,
		DBPath:      filepath.Join(workspaceRoot, ".sesame", "sesame.db"),
		MinPassRate: minPassRate,
	}
	env := &EvalEnv{
		WorkspaceRoot: workspaceRoot,
		DataDir:       filepath.Join(workspaceRoot, ".sesame"),
		DBPath:        report.DBPath,
		Quick:         opts.Quick,
		Long:          opts.Long,
		Verbose:       opts.Verbose,
		HTTPClient:    http.DefaultClient,
		Values:        map[string]any{},
	}

	logf("=== %s ===\n", suite.Name)
	logf("%s\n", suite.Description)
	defer func() {
		if env.PID > 0 {
			if err := StopDaemon(env.PID); err != nil {
				report.Results = append(report.Results, Result("stop daemon", false, err.Error()))
				report.Passed = false
			}
			env.PID = 0
		}
		if !opts.Keep {
			_ = os.RemoveAll(workspaceRoot)
		}
	}()

	pid, baseURL, err := StartDaemon(workspaceRoot)
	if err != nil {
		report.Error = err.Error()
		report.Results = append(report.Results, Result("start daemon", false, err.Error()))
		finalizeSuiteReport(&report)
		return report
	}
	env.PID = pid
	env.BaseURL = baseURL
	logf("Daemon: pid=%d url=%s\n", pid, baseURL)

	sessionID, err := EnsureSessionContext(ctx, baseURL, workspaceRoot)
	if err != nil {
		report.Error = err.Error()
		report.Results = append(report.Results, Result("ensure session", false, err.Error()))
		finalizeSuiteReport(&report)
		return report
	}
	env.SessionID = sessionID
	logf("Session: %s\n", sessionID)

	if suite.Setup != nil {
		if err := suite.Setup(ctx, env); err != nil {
			report.Error = err.Error()
			report.Results = append(report.Results, Result("setup", false, err.Error()))
			finalizeSuiteReport(&report)
			return report
		}
	}

	var results []EvalResult
	if suite.Execute != nil {
		results, err = suite.Execute(ctx, env)
	} else {
		results, err = executeTurns(ctx, env, suite.Turns)
	}
	report.Results = append(report.Results, results...)
	if err != nil {
		report.Error = err.Error()
	}

	if suite.Verify != nil {
		verifyResults, verifyErr := suite.Verify(ctx, env.DBPath)
		report.Results = append(report.Results, verifyResults...)
		if verifyErr != nil {
			if report.Error != "" {
				report.Error += "; " + verifyErr.Error()
			} else {
				report.Error = verifyErr.Error()
			}
		}
	}

	finalizeSuiteReport(&report)
	logf("Result: %.0f%% pass (%d checks), passed=%v\n\n", report.PassRate*100, len(report.Results), report.Passed)
	return report
}

func executeTurns(ctx context.Context, env *EvalEnv, turns []EvalTurn) ([]EvalResult, error) {
	results := make([]EvalResult, 0, len(turns))
	responses := make([]EvalResponse, 0, len(turns))
	for idx, turn := range turns {
		response, err := SendTurnContext(ctx, env.BaseURL, env.SessionID, turn.Message)
		if err != nil {
			results = append(results, Result(fmt.Sprintf("turn %d completed", idx+1), false, err.Error()))
			env.Values["responses"] = responses
			return results, err
		}
		responses = append(responses, response)
		if turn.Validate != nil {
			validated := turn.Validate(response)
			if len(validated) == 0 {
				results = append(results, Result(fmt.Sprintf("turn %d validation", idx+1), true, "no checks returned"))
			} else {
				results = append(results, validated...)
			}
		} else {
			results = append(results, Result(fmt.Sprintf("turn %d completed", idx+1), true, response.TurnID))
		}
	}
	env.Values["responses"] = responses
	return results, nil
}

func finalizeSuiteReport(report *SuiteReport) {
	report.PassRate = PassRate(report.Results)
	report.Passed = report.Error == "" && len(report.Results) > 0 && report.PassRate >= report.MinPassRate
}

func filterSuites(suites []EvalSuite, filter string) ([]EvalSuite, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "all" {
		return suites, nil
	}
	wanted := map[string]struct{}{}
	for _, part := range strings.Split(filter, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			wanted[name] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("suite filter is empty")
	}

	var selected []EvalSuite
	for _, suite := range suites {
		if _, ok := wanted[suite.Name]; ok {
			selected = append(selected, suite)
			delete(wanted, suite.Name)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for name := range wanted {
			missing = append(missing, name)
		}
		return nil, fmt.Errorf("unknown suite(s): %s", strings.Join(missing, ", "))
	}
	return selected, nil
}

func resolveEvalWorkspaceRoot(explicit string) (string, bool, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", false, err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", false, err
		}
		return abs, false, nil
	}
	path, err := os.MkdirTemp("", "sesame-eval-*")
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

func safeSuiteName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = unsafePathChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "suite"
	}
	return name
}
