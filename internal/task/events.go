package task

type Event struct {
	TaskID string `json:"task_id"`
	Type   string `json:"type"`
}
