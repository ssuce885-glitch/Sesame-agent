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

type NotebookEditInput struct {
	NotebookPath string `json:"notebook_path"`
	CellID       string `json:"cell_id,omitempty"`
	NewSource    string `json:"new_source"`
	CellType     string `json:"cell_type,omitempty"`
	EditMode     string `json:"edit_mode,omitempty"`
}

type NotebookEditOutput struct {
	NotebookPath string `json:"notebook_path"`
	CellID       string `json:"cell_id,omitempty"`
	NewSource    string `json:"new_source"`
	CellType     string `json:"cell_type"`
	Language     string `json:"language,omitempty"`
	EditMode     string `json:"edit_mode"`
	OriginalFile string `json:"original_file"`
	UpdatedFile  string `json:"updated_file"`
}

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
				"enum":        []string{"code", "markdown"},
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"description": "replace, insert, or delete.",
				"enum":        []string{"replace", "insert", "delete"},
			},
		}, "notebook_path", "new_source"),
		OutputSchema: objectSchema(map[string]any{
			"notebook_path": map[string]any{"type": "string"},
			"cell_id":       map[string]any{"type": "string"},
			"new_source":    map[string]any{"type": "string"},
			"cell_type":     map[string]any{"type": "string"},
			"language":      map[string]any{"type": "string"},
			"edit_mode":     map[string]any{"type": "string"},
			"original_file": map[string]any{"type": "string"},
			"updated_file":  map[string]any{"type": "string"},
		}, "notebook_path", "new_source", "cell_type", "edit_mode", "original_file", "updated_file"),
	}
}

func (notebookEditTool) IsConcurrencySafe() bool { return false }

func (notebookEditTool) Decode(call Call) (DecodedCall, error) {
	input := NotebookEditInput{
		NotebookPath: strings.TrimSpace(call.StringInput("notebook_path")),
		CellID:       strings.TrimSpace(call.StringInput("cell_id")),
		NewSource:    call.StringInput("new_source"),
		CellType:     strings.TrimSpace(call.StringInput("cell_type")),
		EditMode:     strings.TrimSpace(call.StringInput("edit_mode")),
	}
	if input.NotebookPath == "" {
		return DecodedCall{}, fmt.Errorf("notebook_path is required")
	}
	if input.EditMode == "" {
		input.EditMode = "replace"
	}
	if input.EditMode != "replace" && input.EditMode != "insert" && input.EditMode != "delete" {
		return DecodedCall{}, fmt.Errorf("notebook_edit edit_mode must be replace, insert, or delete")
	}
	if input.CellType != "" && input.CellType != "code" && input.CellType != "markdown" {
		return DecodedCall{}, fmt.Errorf("notebook_edit cell_type must be code or markdown")
	}
	if input.EditMode == "insert" && input.CellType == "" {
		return DecodedCall{}, fmt.Errorf("notebook_edit insert operations require cell_type")
	}
	if input.EditMode != "insert" && input.CellID == "" {
		return DecodedCall{}, fmt.Errorf("notebook_edit %s operations require cell_id", input.EditMode)
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"notebook_path": input.NotebookPath,
			"cell_id":       input.CellID,
			"new_source":    input.NewSource,
			"cell_type":     input.CellType,
			"edit_mode":     input.EditMode,
		},
	}
	return DecodedCall{Call: normalized, Input: input}, nil
}

func (t notebookEditTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (notebookEditTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(NotebookEditInput)
	path := resolveWorkspacePath(execCtx.WorkspaceRoot, input.NotebookPath)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return ToolExecutionResult{}, err
	}
	if filepath.Ext(path) != ".ipynb" {
		return ToolExecutionResult{}, fmt.Errorf("notebook_edit requires a .ipynb file")
	}

	info, err := os.Stat(path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	originalText := string(data)

	var notebook map[string]any
	if err := json.Unmarshal(data, &notebook); err != nil {
		return ToolExecutionResult{}, fmt.Errorf("notebook_edit could not parse notebook JSON: %w", err)
	}

	cells, err := notebookCells(notebook)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	resolvedCellID := input.CellID
	resolvedCellType := input.CellType
	language := notebookLanguage(notebook)

	switch input.EditMode {
	case "replace":
		cellIndex, ok := findNotebookCellIndex(cells, input.CellID)
		if !ok {
			return ToolExecutionResult{}, fmt.Errorf("notebook_edit could not find cell %q", input.CellID)
		}
		target := cells[cellIndex]
		if resolvedCellType == "" {
			resolvedCellType, _ = target["cell_type"].(string)
		}
		target["source"] = input.NewSource
		target["cell_type"] = resolvedCellType
		target["metadata"] = notebookCellMetadata(target["metadata"])
		if resolvedCellType == "code" {
			target["execution_count"] = nil
			target["outputs"] = []any{}
		} else {
			delete(target, "execution_count")
			delete(target, "outputs")
		}
	case "insert":
		insertIndex := 0
		if input.CellID != "" {
			cellIndex, ok := findNotebookCellIndex(cells, input.CellID)
			if !ok {
				return ToolExecutionResult{}, fmt.Errorf("notebook_edit could not find cell %q", input.CellID)
			}
			insertIndex = cellIndex + 1
		}

		resolvedCellID = nextNotebookCellID(cells)
		newCell := map[string]any{
			"cell_type": resolvedCellType,
			"id":        resolvedCellID,
			"source":    input.NewSource,
			"metadata":  map[string]any{},
		}
		if resolvedCellType == "code" {
			newCell["execution_count"] = nil
			newCell["outputs"] = []any{}
		}

		cells = append(cells, nil)
		copy(cells[insertIndex+1:], cells[insertIndex:])
		cells[insertIndex] = newCell
	case "delete":
		cellIndex, ok := findNotebookCellIndex(cells, input.CellID)
		if !ok {
			return ToolExecutionResult{}, fmt.Errorf("notebook_edit could not find cell %q", input.CellID)
		}
		if resolvedCellType == "" {
			resolvedCellType, _ = cells[cellIndex]["cell_type"].(string)
		}
		cells = append(cells[:cellIndex], cells[cellIndex+1:]...)
	}

	setNotebookCells(notebook, cells)

	updated, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return ToolExecutionResult{}, err
	}
	updated = append(updated, '\n')
	updatedText := string(updated)

	if err := os.WriteFile(path, updated, info.Mode().Perm()); err != nil {
		return ToolExecutionResult{}, err
	}

	modelText := notebookEditModelText(input.EditMode, resolvedCellID, path)
	return ToolExecutionResult{
		Result: Result{
			Text:      path,
			ModelText: modelText,
		},
		Data: NotebookEditOutput{
			NotebookPath: path,
			CellID:       resolvedCellID,
			NewSource:    input.NewSource,
			CellType:     resolvedCellType,
			Language:     language,
			EditMode:     input.EditMode,
			OriginalFile: originalText,
			UpdatedFile:  updatedText,
		},
		PreviewText: modelText,
		Metadata: map[string]any{
			"cell_id":   resolvedCellID,
			"cell_type": resolvedCellType,
			"edit_mode": input.EditMode,
			"language":  language,
		},
	}, nil
}

func (notebookEditTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
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

func notebookLanguage(notebook map[string]any) string {
	metadata, _ := notebook["metadata"].(map[string]any)
	if metadata == nil {
		return ""
	}
	if languageInfo, ok := metadata["language_info"].(map[string]any); ok {
		if name, _ := languageInfo["name"].(string); strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	if kernelspec, ok := metadata["kernelspec"].(map[string]any); ok {
		if language, _ := kernelspec["language"].(string); strings.TrimSpace(language) != "" {
			return strings.TrimSpace(language)
		}
	}
	return ""
}

func notebookEditModelText(editMode, cellID, path string) string {
	switch editMode {
	case "replace":
		return fmt.Sprintf("Updated notebook cell %s in %s", cellID, path)
	case "insert":
		return fmt.Sprintf("Inserted notebook cell %s in %s", cellID, path)
	case "delete":
		return fmt.Sprintf("Deleted notebook cell %s from %s", cellID, path)
	default:
		return fmt.Sprintf("Updated notebook %s", path)
	}
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
