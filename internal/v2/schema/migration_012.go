package schema

var Migration012 = Migration{
	Version: 12,
	Name:    "workflow_run_dedupe_ref_uniqueness",
	Up: `
ALTER TABLE v2_workflow_runs
ADD COLUMN dedupe_ref TEXT NOT NULL DEFAULT '';

WITH ranked AS (
    SELECT
        id,
        trigger_ref,
        ROW_NUMBER() OVER (
            PARTITION BY workflow_id, trigger_ref
            ORDER BY created_at ASC, id ASC
        ) AS row_num
    FROM v2_workflow_runs
    WHERE trigger_ref LIKE 'automation:%'
)
UPDATE v2_workflow_runs
SET dedupe_ref = trigger_ref
WHERE id IN (
    SELECT id
    FROM ranked
    WHERE row_num = 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_v2_workflow_runs_workflow_dedupe_ref
ON v2_workflow_runs(workflow_id, dedupe_ref)
WHERE dedupe_ref <> '';
`,
}
