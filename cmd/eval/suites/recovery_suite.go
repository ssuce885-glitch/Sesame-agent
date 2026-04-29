package suites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/cmd/eval/internal/evalcore"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

func RecoverySuite(evalcore.SuiteOptions) evalcore.EvalSuite {
	return evalcore.EvalSuite{
		Name:        "recovery",
		Description: "Kills the daemon during an active turn, restarts it, and verifies terminal turn state plus sqlite integrity.",
		Execute:     executeRecoverySuite,
		MinPassRate: 1.0,
	}
}

func executeRecoverySuite(ctx context.Context, env *evalcore.EvalEnv) ([]evalcore.EvalResult, error) {
	turn, err := evalcore.SubmitTurn(ctx, env.BaseURL, "Run shell_command `find . -maxdepth 2 -type f -print | sort; sleep 15; echo RECOVERY_EVAL_DONE` and summarize the output.")
	if err != nil {
		return []evalcore.EvalResult{evalcore.Result("submit recovery turn", false, err.Error())}, err
	}
	results := []evalcore.EvalResult{
		evalcore.Result("submit recovery turn", true, turn.ID),
	}

	stateBeforeKill, err := evalcore.WaitForTurnStateChange(ctx, env.DBPath, turn.ID, types.TurnStateCreated, 5*time.Second)
	if err != nil {
		return append(results, evalcore.Result("observe pre-crash turn state", false, err.Error())), err
	}
	results = append(results, evalcore.Result("observe pre-crash turn state", true, string(stateBeforeKill)))

	oldPID := env.PID
	if err := evalcore.StopDaemon(oldPID); err != nil {
		return append(results, evalcore.Result("kill daemon mid-turn", false, err.Error())), err
	}
	env.PID = 0
	results = append(results, evalcore.Result("kill daemon mid-turn", true, fmt.Sprintf("pid=%d", oldPID)))

	pid, baseURL, err := evalcore.StartDaemon(env.WorkspaceRoot)
	if err != nil {
		return append(results, evalcore.Result("restart daemon", false, err.Error())), err
	}
	env.PID = pid
	env.BaseURL = baseURL
	results = append(results, evalcore.Result("restart daemon", true, fmt.Sprintf("pid=%d", pid)))

	sessionID, err := evalcore.EnsureSessionContext(ctx, env.BaseURL, env.WorkspaceRoot)
	if err != nil {
		return append(results, evalcore.Result("ensure session after restart", false, err.Error())), err
	}
	results = append(results, evalcore.Result("ensure session after restart", strings.TrimSpace(sessionID) != "", sessionID))
	env.SessionID = sessionID

	finalState, waitErr := evalcore.WaitForTurnTerminal(ctx, env.DBPath, turn.ID, 180*time.Second)
	if waitErr != nil {
		results = append(results, evalcore.Result("turn recovered to terminal state", false, waitErr.Error()))
	} else {
		recovered := finalState == types.TurnStateCompleted || finalState == types.TurnStateInterrupted
		results = append(results, evalcore.Result("turn recovered to expected state", recovered, string(finalState)))
	}

	results = append(results, evalcore.SQLiteIntegrityOK(ctx, env.DBPath))
	consistent, detail, err := recoverySessionConsistent(ctx, env.DBPath)
	if err != nil {
		return append(results, evalcore.Result("session state consistent", false, err.Error())), err
	}
	results = append(results, evalcore.Result("session state consistent", consistent, detail))
	return results, nil
}

func recoverySessionConsistent(ctx context.Context, dbPath string) (bool, string, error) {
	store, err := sqlite.Open(dbPath)
	if err != nil {
		return false, "", err
	}
	defer store.Close()

	var runningTurns int
	if err := store.DB().QueryRowContext(ctx, `
		select count(*)
		from turns
		where state in (?, ?, ?, ?, ?)
	`, types.TurnStateBuildingContext, types.TurnStateModelStreaming, types.TurnStateToolDispatching, types.TurnStateToolRunning, types.TurnStateLoopContinue).Scan(&runningTurns); err != nil {
		return false, "", err
	}

	var activeSessions int
	if err := store.DB().QueryRowContext(ctx, `
		select count(*)
		from sessions
		where active_turn_id != ''
	`).Scan(&activeSessions); err != nil {
		return false, "", err
	}
	return runningTurns == 0 && activeSessions == 0, fmt.Sprintf("running_turns=%d active_sessions=%d", runningTurns, activeSessions), nil
}
