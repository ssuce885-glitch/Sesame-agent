package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/model"
)

const viewImageMaxBytes = 10 << 20

type viewImageTool struct{}

type ViewImageInput struct {
	Path string `json:"path"`
}

type ViewImageOutput struct {
	Path      string `json:"path"`
	MimeType  string `json:"mime_type"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

func (viewImageTool) Definition() Definition {
	return Definition{
		Name:        "view_image",
		Description: "Load a local image into the model context for multimodal reasoning.",
		InputSchema: objectSchema(map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to an image file in the workspace or under Sesame's global config directory.",
			},
		}, "path"),
		OutputSchema: objectSchema(map[string]any{
			"path":       map[string]any{"type": "string"},
			"mime_type":  map[string]any{"type": "string"},
			"width":      map[string]any{"type": "integer"},
			"height":     map[string]any{"type": "integer"},
			"size_bytes": map[string]any{"type": "integer"},
		}, "path", "mime_type", "size_bytes"),
	}
}

func (viewImageTool) IsConcurrencySafe() bool { return true }

func (viewImageTool) Decode(call Call) (DecodedCall, error) {
	path := strings.TrimSpace(call.StringInput("path"))
	if path == "" {
		return DecodedCall{}, fmt.Errorf("path is required")
	}
	normalized := Call{
		ID:   call.ID,
		Name: call.Name,
		Input: map[string]any{
			"path": path,
		},
	}
	return DecodedCall{Call: normalized, Input: ViewImageInput{Path: path}}, nil
}

func (t viewImageTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (viewImageTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(ViewImageInput)
	resolvedPath, err := resolveReadablePath(execCtx, input.Path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if len(data) > viewImageMaxBytes {
		return ToolExecutionResult{}, fmt.Errorf("image exceeds max size (%d bytes)", viewImageMaxBytes)
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return ToolExecutionResult{}, fmt.Errorf("path is not a supported image")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		cfg = image.Config{}
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	displayPath := input.Path
	if rel, err := filepath.Rel(execCtx.WorkspaceRoot, resolvedPath); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		displayPath = filepath.ToSlash(rel)
	}

	message := fmt.Sprintf("Loaded image %s (%s, %d bytes).", displayPath, mimeType, len(data))
	return ToolExecutionResult{
		Result: Result{
			Text:      message,
			ModelText: message,
		},
		Data: ViewImageOutput{
			Path:      resolvedPath,
			MimeType:  mimeType,
			Width:     cfg.Width,
			Height:    cfg.Height,
			SizeBytes: int64(len(data)),
		},
		PreviewText: message,
		NewItems: []model.ConversationItem{
			model.UserMultipartItem([]model.ContentPart{
				{
					Type: model.ContentPartText,
					Text: "Local image attachment: " + displayPath,
				},
				{
					Type:       model.ContentPartImage,
					MimeType:   mimeType,
					DataBase64: encoded,
					Path:       displayPath,
					Width:      cfg.Width,
					Height:     cfg.Height,
					SizeBytes:  int64(len(data)),
				},
			}),
		},
		Metadata: map[string]any{
			"mime_type":  mimeType,
			"path":       displayPath,
			"width":      cfg.Width,
			"height":     cfg.Height,
			"size_bytes": len(data),
		},
	}, nil
}

func (viewImageTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
