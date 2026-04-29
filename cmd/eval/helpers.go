package main

import "go-agent/cmd/eval/internal/evalcore"

func startDaemon(workspaceRoot string) (pid int, url string, err error) {
	return evalcore.StartDaemon(workspaceRoot)
}

func stopDaemon(pid int) error {
	return evalcore.StopDaemon(pid)
}

func ensureSession(baseURL, workspaceRoot string) (sessionID string, err error) {
	return evalcore.EnsureSession(baseURL, workspaceRoot)
}

func sendTurn(baseURL, sessionID, message string) (EvalResponse, error) {
	return evalcore.SendTurn(baseURL, sessionID, message)
}
