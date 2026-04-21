package tools

func automationSpecInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           automationSpecProperties(),
		"additionalProperties": false,
	}
}

func automationSpecOutputSchema() map[string]any {
	return objectSchema(automationSpecProperties(), "id", "title", "workspace_root", "goal", "state")
}

func automationAssetSchema() map[string]any {
	return objectSchema(map[string]any{
		"path": map[string]any{
			"type": "string",
		},
		"content": map[string]any{
			"type": "string",
		},
		"executable": map[string]any{
			"type": "boolean",
		},
	}, "path", "content")
}

func automationSpecProperties() map[string]any {
	return map[string]any{
		"id": map[string]any{
			"type": "string",
		},
		"title": map[string]any{
			"type": "string",
		},
		"workspace_root": map[string]any{
			"type": "string",
		},
		"goal": map[string]any{
			"type": "string",
		},
		"state": map[string]any{
			"type": "string",
			"enum": automationStateEnum(),
		},
		"context": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"targets": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"labels": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
				"owner": map[string]any{
					"type": "string",
				},
				"environment": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": false,
		},
		"signals": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
					},
					"source": map[string]any{
						"type": "string",
					},
					"selector": map[string]any{
						"type": "string",
					},
					"payload": map[string]any{},
				},
				"additionalProperties": false,
			},
		},
		"incident_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"response_plan": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"verification_plan": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"escalation_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"delivery_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"runtime_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"watcher_lifecycle": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"retrigger_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"run_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"assumptions": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"created_at": map[string]any{
			"type": "string",
		},
		"updated_at": map[string]any{
			"type": "string",
		},
	}
}

func automationIncidentOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"id": map[string]any{
			"type": "string",
		},
		"automation_id": map[string]any{
			"type": "string",
		},
		"workspace_root": map[string]any{
			"type": "string",
		},
		"status": map[string]any{
			"type": "string",
			"enum": automationIncidentStatusEnum(),
		},
		"signal_kind": map[string]any{
			"type": "string",
		},
		"source": map[string]any{
			"type": "string",
		},
		"summary": map[string]any{
			"type": "string",
		},
		"payload": map[string]any{},
		"observed_at": map[string]any{
			"type": "string",
		},
		"created_at": map[string]any{
			"type": "string",
		},
		"updated_at": map[string]any{
			"type": "string",
		},
	}, "id", "automation_id", "workspace_root", "status")
}
