package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	requestUserInputMinOptions = 2
	requestUserInputMaxOptions = 3
	requestUserInputMaxPrompts = 3
)

type requestUserInputTool struct{}

type RequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type RequestUserInputQuestion struct {
	ID       string                   `json:"id"`
	Header   string                   `json:"header"`
	Question string                   `json:"question"`
	Options  []RequestUserInputOption `json:"options"`
}

type RequestUserInputInput struct {
	Questions []RequestUserInputQuestion `json:"questions"`
}

type RequestUserInputOutput struct {
	Status     string                     `json:"status"`
	Questions  []RequestUserInputQuestion `json:"questions"`
	PromptText string                     `json:"prompt_text"`
}

func (requestUserInputTool) Definition() Definition {
	optionSchema := objectSchema(map[string]any{
		"label": map[string]any{
			"type":        "string",
			"description": "User-facing label (1-5 words).",
		},
		"description": map[string]any{
			"type":        "string",
			"description": "One short sentence explaining impact or tradeoff if selected.",
		},
	}, "label", "description")

	questionSchema := objectSchema(map[string]any{
		"id": map[string]any{
			"type":        "string",
			"description": "Stable identifier for mapping answers.",
		},
		"header": map[string]any{
			"type":        "string",
			"description": "Short header label shown in the UI.",
		},
		"question": map[string]any{
			"type":        "string",
			"description": "Single-sentence prompt shown to the user.",
		},
		"options": map[string]any{
			"type":        "array",
			"description": "Provide 2-3 mutually exclusive choices; put the recommended option first.",
			"items":       optionSchema,
		},
	}, "id", "header", "question", "options")

	return Definition{
		Name:        "request_user_input",
		Description: "Request short structured user input and pause the current turn until the user replies.",
		InputSchema: objectSchema(map[string]any{
			"questions": map[string]any{
				"type":        "array",
				"description": "Questions to show the user. Prefer one and do not exceed three.",
				"items":       questionSchema,
			},
		}, "questions"),
		OutputSchema: objectSchema(map[string]any{
			"status": map[string]any{"type": "string"},
			"questions": map[string]any{
				"type":  "array",
				"items": questionSchema,
			},
			"prompt_text": map[string]any{"type": "string"},
		}, "status", "questions", "prompt_text"),
	}
}

func (requestUserInputTool) IsConcurrencySafe() bool { return false }

func (requestUserInputTool) Decode(call Call) (DecodedCall, error) {
	if call.Input == nil {
		call.Input = map[string]any{}
	}

	var input RequestUserInputInput
	raw, err := json.Marshal(call.Input)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("request_user_input input could not be decoded")
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return DecodedCall{}, fmt.Errorf("request_user_input input could not be decoded")
	}
	if len(input.Questions) == 0 {
		return DecodedCall{}, fmt.Errorf("questions is required")
	}
	if len(input.Questions) > requestUserInputMaxPrompts {
		return DecodedCall{}, fmt.Errorf("request_user_input supports at most %d questions", requestUserInputMaxPrompts)
	}

	seenIDs := make(map[string]struct{}, len(input.Questions))
	for i := range input.Questions {
		question := &input.Questions[i]
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" {
			return DecodedCall{}, fmt.Errorf("questions[%d].id is required", i)
		}
		if _, exists := seenIDs[question.ID]; exists {
			return DecodedCall{}, fmt.Errorf("duplicate question id %q", question.ID)
		}
		seenIDs[question.ID] = struct{}{}
		if question.Header == "" {
			return DecodedCall{}, fmt.Errorf("questions[%d].header is required", i)
		}
		if question.Question == "" {
			return DecodedCall{}, fmt.Errorf("questions[%d].question is required", i)
		}
		if len(question.Options) < requestUserInputMinOptions || len(question.Options) > requestUserInputMaxOptions {
			return DecodedCall{}, fmt.Errorf("questions[%d].options must contain %d-%d choices", i, requestUserInputMinOptions, requestUserInputMaxOptions)
		}
		for j := range question.Options {
			option := &question.Options[j]
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				return DecodedCall{}, fmt.Errorf("questions[%d].options[%d].label is required", i, j)
			}
			if option.Description == "" {
				return DecodedCall{}, fmt.Errorf("questions[%d].options[%d].description is required", i, j)
			}
		}
	}

	normalized := Call{
		Name:  call.Name,
		Input: map[string]any{"questions": input.Questions},
	}
	return DecodedCall{
		Call:  normalized,
		Input: input,
	}, nil
}

func (t requestUserInputTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (requestUserInputTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, _ ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(RequestUserInputInput)
	promptText := renderUserInputPrompt(input.Questions)
	modelText := strings.TrimSpace(
		"The tool requested user input and paused the current turn. Wait for the user's next response before taking additional actions.\n\n" +
			promptText,
	)

	return ToolExecutionResult{
		Result: Result{
			Text:      promptText,
			ModelText: modelText,
		},
		Data: RequestUserInputOutput{
			Status:     "awaiting_user_input",
			Questions:  input.Questions,
			PromptText: promptText,
		},
		PreviewText: fmt.Sprintf("Requested user input (%d question(s))", len(input.Questions)),
		Interrupt: &ToolInterrupt{
			Reason: "user_input_requested",
			Notice: promptText,
		},
	}, nil
}

func (requestUserInputTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func renderUserInputPrompt(questions []RequestUserInputQuestion) string {
	lines := []string{"Additional user input is required before continuing:"}
	for i, question := range questions {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, question.Header, question.Question))
		for j, option := range question.Options {
			lines = append(lines, fmt.Sprintf("   %d) %s — %s", j+1, option.Label, option.Description))
		}
	}
	return strings.Join(lines, "\n")
}
