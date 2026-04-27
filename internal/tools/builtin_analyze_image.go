package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go-agent/internal/config"
)

const (
	defaultAnalyzeImagePrompt = "请描述这张图片"
	analyzeImageMaxTokens     = 1024
	analyzeImageTimeout       = 60 * time.Second
)

type analyzeImageTool struct{}

type AnalyzeImageInput struct {
	ImagePath string `json:"image_path"`
	ImageURL  string `json:"image_url"`
	Prompt    string `json:"prompt"`
}

type AnalyzeImageOutput struct {
	Description string `json:"description"`
	Model       string `json:"model"`
	Provider    string `json:"provider"`
}

type chatCompletionsRequest struct {
	Model     string                   `json:"model"`
	Messages  []chatCompletionsMessage `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
}

type chatCompletionsMessage struct {
	Role    string                   `json:"role"`
	Content []chatCompletionsContent `json:"content"`
}

type chatCompletionsContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatCompletionsResponse struct {
	Choices []chatCompletionsChoice `json:"choices"`
	Error   *chatCompletionsError   `json:"error,omitempty"`
}

type chatCompletionsChoice struct {
	Message chatCompletionsResponseMessage `json:"message"`
}

type chatCompletionsResponseMessage struct {
	Content string `json:"content"`
}

type chatCompletionsError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

func (analyzeImageTool) Definition() Definition {
	return Definition{
		Name:        "analyze_image",
		Description: "Analyze an image using a separately configured vision model. Provide either a local file path or a remote image URL.",
		InputSchema: objectSchema(map[string]any{
			"image_path": map[string]any{
				"type":        "string",
				"description": "Optional local image path. Used before image_url when both are provided.",
			},
			"image_url": map[string]any{
				"type":        "string",
				"description": "Optional remote http:// or https:// image URL.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Optional prompt for the vision model.",
				"default":     defaultAnalyzeImagePrompt,
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"description": map[string]any{"type": "string"},
			"model":       map[string]any{"type": "string"},
			"provider":    map[string]any{"type": "string"},
		}, "description", "model", "provider"),
	}
}

func (analyzeImageTool) IsConcurrencySafe() bool { return true }

func (analyzeImageTool) Decode(call Call) (DecodedCall, error) {
	imagePath := strings.TrimSpace(call.StringInput("image_path"))
	imageURL := strings.TrimSpace(call.StringInput("image_url"))
	prompt := strings.TrimSpace(call.StringInput("prompt"))
	if prompt == "" {
		prompt = defaultAnalyzeImagePrompt
	}
	if imagePath == "" && imageURL == "" {
		return DecodedCall{}, fmt.Errorf("image_path or image_url is required")
	}
	if imagePath == "" {
		normalizedURL, err := normalizeAnalyzeImageURL(imageURL)
		if err != nil {
			return DecodedCall{}, err
		}
		imageURL = normalizedURL
	}

	normalizedInput := map[string]any{
		"prompt": prompt,
	}
	if imagePath != "" {
		normalizedInput["image_path"] = imagePath
	}
	if imageURL != "" {
		normalizedInput["image_url"] = imageURL
	}

	return DecodedCall{
		Call: Call{
			ID:    call.ID,
			Name:  call.Name,
			Input: normalizedInput,
		},
		Input: AnalyzeImageInput{
			ImagePath: imagePath,
			ImageURL:  imageURL,
			Prompt:    prompt,
		},
	}, nil
}

func (t analyzeImageTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (analyzeImageTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(AnalyzeImageInput)
	if input.Prompt == "" {
		input.Prompt = defaultAnalyzeImagePrompt
	}
	if strings.TrimSpace(input.ImagePath) == "" && strings.TrimSpace(input.ImageURL) == "" {
		return ToolExecutionResult{}, fmt.Errorf("image_path or image_url is required")
	}

	visionCfg, err := config.LoadUserConfigFromGlobalRoot(execCtx.GlobalConfigRoot)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("load vision config: %w", err)
	}

	provider := analyzeImageConfigValue(execCtx, "SESAME_VISION_PROVIDER", visionCfg.Vision.Provider)
	apiKey := analyzeImageConfigValue(execCtx, "SESAME_VISION_API_KEY", visionCfg.Vision.APIKey)
	baseURL := analyzeImageConfigValue(execCtx, "SESAME_VISION_BASE_URL", visionCfg.Vision.BaseURL)
	model := analyzeImageConfigValue(execCtx, "SESAME_VISION_MODEL", visionCfg.Vision.Model)
	if provider == "" || model == "" {
		return ToolExecutionResult{}, fmt.Errorf("vision configuration is not configured: set vision.provider and vision.model")
	}
	if apiKey == "" {
		return ToolExecutionResult{}, fmt.Errorf("vision api_key is not configured")
	}
	if baseURL == "" {
		return ToolExecutionResult{}, fmt.Errorf("vision base_url is not configured")
	}

	imageRef := strings.TrimSpace(input.ImageURL)
	if strings.TrimSpace(input.ImagePath) != "" {
		imageRef, err = analyzeImageDataURL(execCtx, input.ImagePath)
		if err != nil {
			return ToolExecutionResult{}, err
		}
	} else {
		imageRef, err = normalizeAnalyzeImageURL(imageRef)
		if err != nil {
			return ToolExecutionResult{}, err
		}
	}

	description, err := callChatCompletionsVision(ctx, baseURL, apiKey, model, input.Prompt, imageRef)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	output := AnalyzeImageOutput{
		Description: description,
		Model:       model,
		Provider:    provider,
	}
	modelText := truncateAnalyzeImageText(description, 500)
	return ToolExecutionResult{
		Result: Result{
			Text:      description,
			ModelText: modelText,
		},
		Data:        output,
		PreviewText: "Analyzed image: " + truncateAnalyzeImageText(description, 80),
		Metadata: map[string]any{
			"model":    model,
			"provider": provider,
		},
	}, nil
}

func (analyzeImageTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func analyzeImageConfigValue(execCtx ExecContext, key, fileValue string) string {
	if execCtx.InjectedEnv != nil {
		if value := strings.TrimSpace(execCtx.InjectedEnv[key]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fileValue)
}

func analyzeImageDataURL(execCtx ExecContext, path string) (string, error) {
	resolvedPath, err := resolveReadablePath(execCtx, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", err
	}
	if len(data) > viewImageMaxBytes {
		return "", fmt.Errorf("image exceeds max size (%d bytes)", viewImageMaxBytes)
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("path is not a supported image")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

func normalizeAnalyzeImageURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return "", fmt.Errorf("image_url must be a valid http or https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("image_url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("image_url host is required")
	}
	return parsed.String(), nil
}

func callChatCompletionsVision(ctx context.Context, baseURL, apiKey, model, prompt, imageRef string) (string, error) {
	payload := chatCompletionsRequest{
		Model: model,
		Messages: []chatCompletionsMessage{
			{
				Role: "user",
				Content: []chatCompletionsContent{
					{
						Type: "text",
						Text: prompt,
					},
					{
						Type: "image_url",
						ImageURL: &imageURL{
							URL: imageRef,
						},
					},
				},
			},
		},
		MaxTokens: analyzeImageMaxTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal vision request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, analyzeImageTimeout)
	defer cancel()

	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build vision request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: analyzeImageTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call vision API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read vision response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("vision API request failed with status %s: %s", resp.Status, renderChatCompletionsError(respBody))
	}

	var completion chatCompletionsResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return "", fmt.Errorf("parse vision response: %w", err)
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return "", fmt.Errorf("vision API error: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("vision API response did not include choices")
	}

	description := strings.TrimSpace(completion.Choices[0].Message.Content)
	if description == "" {
		return "", fmt.Errorf("vision API response did not include message content")
	}
	return description, nil
}

func renderChatCompletionsError(body []byte) string {
	var completion chatCompletionsResponse
	if err := json.Unmarshal(body, &completion); err == nil && completion.Error != nil {
		if message := strings.TrimSpace(completion.Error.Message); message != "" {
			return message
		}
	}
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return "empty response body"
	}
	return truncateAnalyzeImageText(detail, 512)
}

func truncateAnalyzeImageText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
