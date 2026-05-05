package schema

var Migration011 = Migration{
	Version: 11,
	Name:    "automation_workflow_links",
	Up: `
ALTER TABLE v2_automations ADD COLUMN workflow_id TEXT NOT NULL DEFAULT '';
ALTER TABLE v2_automation_runs ADD COLUMN workflow_run_id TEXT NOT NULL DEFAULT '';
`,
}
