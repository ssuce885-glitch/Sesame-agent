package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go-agent/internal/runtime"
)

type notebookEditTool struct{}

func (notebookEditTool) Definition() Definition {
	return Definition{
		Name:        "notebook_edit",
		Description: "Edit cells in a Jupyter notebook.",
		InputSchema: objectSchema(map[string]any{
			"notebook_path": map[string]any{
				"type":        "string",
				"description": "Path to the .ipynb file to edit.",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "Cell identifier to replace, insert after, or delete.",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "New cell source content.",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"description": "Cell type for inserts or type changes.",
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"description": "replace, insert, or delete.",
			},
		}, "notebook_path", "new_source"),
	}
}

func (notebookEditTool) IsConcurrencySafe() bool { return false }

func (notebookEditTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.StringInput("notebook_path")
	cellID := call.StringInput("cell_id")
	newSource := call.StringInput("new_source")
	cellType := call.StringInput("cell_type")
	editMode := call.StringInput("edit_mode")
	if editMode == "" {
		editMode = "replace"
	}

	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}
	if filepath.Ext(path) != ".ipynb" {
		return Result{}, fmt.Errorf("notebook_edit requires a .ipynb file")
	}
	if editMode != "replace" && editMode != "insert" && editMode != "delete" {
		return Result{}, fmt.Errorf("notebook_edit edit_mode must be replace, insert, or delete")
	}
	if cellType != "" && cellType != "code" && cellType != "markdown" {
		return Result{}, fmt.Errorf("notebook_edit cell_type must be code or markdown")
	}
	if editMode == "insert" && cellType == "" {
		return Result{}, fmt.Errorf("notebook_edit insert operations require cell_type")
	}
	if editMode != "insert" && cellID == "" {
		return Result{}, fmt.Errorf("notebook_edit %s operations require cell_id", editMode)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}

	var notebook map[string]any
	if err := json.Unmarshal(data, &notebook); err != nil {
		return Result{}, fmt.Errorf("notebook_edit could not parse notebook JSON: %w", err)
	}

	cells, err := notebookCells(notebook)
	if err != nil {
		return Result{}, err
	}

	switch editMode {
	case "replace":
		cellIndex, ok := findNotebookCellIndex(cells, cellID)
		if !ok {
			return Result{}, fmt.Errorf("notebook_edit could not find cell %q", cellID)
		}
		target := cells[cellIndex]
		resolvedType := cellType
		if resolvedType == "" {
			resolvedType, _ = target["cell_type"].(string)
		}
		target["source"] = newSource
		target["cell_type"] = resolvedType
		target["metadata"] = notebookCellMetadata(target["metadata"])
		if resolvedType == "code" {
			target["execution_count"] = nil
			target["outputs"] = []any{}
		} else {
			delete(target, "execution_count")
			delete(target, "outputs")
		}
	case "insert":
		insertIndex := 0
		if cellID != "" {
			cellIndex, ok := findNotebookCellIndex(cells, cellID)
			if !ok {
				return Result{}, fmt.Errorf("notebook_edit could not find cell %q", cellID)
			}
			insertIndex = cellIndex + 1
		}

		newCell := map[string]any{
			"cell_type": cellType,
			"id":        nextNotebookCellID(cells),
			"source":    newSource,
			"metadata":  map[string]any{},
		}
		if cellType == "code" {
			newCell["execution_count"] = nil
			newCell["outputs"] = []any{}
		}

		cells = append(cells, nil)
		copy(cells[insertIndex+1:], cells[insertIndex:])
		cells[insertIndex] = newCell
	case "delete":
		cellIndex, ok := findNotebookCellIndex(cells, cellID)
		if !ok {
			return Result{}, fmt.Errorf("notebook_edit could not find cell %q", cellID)
		}
		cells = append(cells[:cellIndex], cells[cellIndex+1:]...)
	}

	setNotebookCells(notebook, cells)

	updated, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return Result{}, err
	}
	updated = append(updated, '\n')

	if err := os.WriteFile(path, updated, info.Mode().Perm()); err != nil {
		return Result{}, err
	}

	return Result{Text: path}, nil
}

func notebookCells(notebook map[string]any) ([]map[string]any, error) {
	rawCells, ok := notebook["cells"].([]any)
	if !ok {
		return nil, fmt.Errorf("notebook_edit notebook is missing a valid cells array")
	}

	cells := make([]map[string]any, 0, len(rawCells))
	for _, rawCell := range rawCells {
		cell, ok := rawCell.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("notebook_edit notebook contains an invalid cell")
		}
		cells = append(cells, cell)
	}

	return cells, nil
}

func setNotebookCells(notebook map[string]any, cells []map[string]any) {
	serialized := make([]any, 0, len(cells))
	for _, cell := range cells {
		serialized = append(serialized, cell)
	}
	notebook["cells"] = serialized
}

func notebookCellMetadata(raw any) map[string]any {
	metadata, ok := raw.(map[string]any)
	if ok && metadata != nil {
		return metadata
	}
	return map[string]any{}
}

func findNotebookCellIndex(cells []map[string]any, cellID string) (int, bool) {
	for index, cell := range cells {
		if id, _ := cell["id"].(string); id == cellID {
			return index, true
		}
	}

	if parsed, ok := parseNotebookCellReference(cellID); ok && parsed >= 0 && parsed < len(cells) {
		return parsed, true
	}

	return 0, false
}

func parseNotebookCellReference(cellID string) (int, bool) {
	if !strings.HasPrefix(cellID, "cell-") {
		return 0, false
	}

	index, err := strconv.Atoi(strings.TrimPrefix(cellID, "cell-"))
	if err != nil {
		return 0, false
	}
	return index, true
}

func nextNotebookCellID(cells []map[string]any) string {
	used := make(map[string]struct{}, len(cells))
	for _, cell := range cells {
		if id, _ := cell["id"].(string); id != "" {
			used[id] = struct{}{}
		}
	}

	for index := len(cells); ; index++ {
		candidate := fmt.Sprintf("cell-%d", index)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}
