package schema

var Migration006 = Migration{
	Version: 6,
	Name:    "task_role_id",
	Up: `
ALTER TABLE v2_tasks ADD COLUMN role_id TEXT NOT NULL DEFAULT '';
`,
}
