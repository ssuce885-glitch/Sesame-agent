package task

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type tasksFilePayload struct {
	Tasks []Task `json:"tasks"`
}

func loadTasksFile(path string) (tasksFilePayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tasksFilePayload{}, nil
		}
		return tasksFilePayload{}, err
	}

	if strings.TrimSpace(string(data)) == "" {
		return tasksFilePayload{}, nil
	}

	var payload tasksFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return tasksFilePayload{}, err
	}

	return payload, nil
}

func writeTasksFile(path string, payload tasksFilePayload) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTodosFile(path string, todos []TodoItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}
