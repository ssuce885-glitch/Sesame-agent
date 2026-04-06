package task

import (
	"encoding/json"
	"errors"
	"os"
)

type tasksFilePayload struct {
	Tasks []Task `json:"tasks"`
}

func loadTasksFile(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var payload tasksFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	return payload.Tasks, nil
}

func writeTasksFile(path string, tasks []Task) error {
	payload := tasksFilePayload{Tasks: tasks}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTodosFile(path string, todos []TodoItem) error {
	data, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}
