package tools

type Definition struct {
	Name           string
	Aliases        []string
	Description    string
	InputSchema    map[string]any
	OutputSchema   map[string]any
	MaxInlineBytes int
}

func cloneDefinition(def Definition) Definition {
	return Definition{
		Name:           def.Name,
		Aliases:        cloneStringSlice(def.Aliases),
		Description:    def.Description,
		InputSchema:    cloneStringAnyMap(def.InputSchema),
		OutputSchema:   cloneStringAnyMap(def.OutputSchema),
		MaxInlineBytes: def.MaxInlineBytes,
	}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneStringSlice(src []string) []string {
	if src == nil {
		return nil
	}

	cloned := make([]string, len(src))
	copy(cloned, src)
	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []string:
		cloned := make([]string, len(typed))
		copy(cloned, typed)
		return cloned
	case []map[string]any:
		cloned := make([]map[string]any, len(typed))
		for i, elem := range typed {
			cloned[i] = cloneStringAnyMap(elem)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, elem := range typed {
			cloned[i] = cloneAny(elem)
		}
		return cloned
	default:
		return value
	}
}
