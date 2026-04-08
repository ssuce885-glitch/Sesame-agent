package tools

import (
	"fmt"
	"strings"
)

func validateInputSchema(schema map[string]any, input map[string]any) error {
	if schema == nil {
		return nil
	}
	if input == nil {
		input = map[string]any{}
	}
	return validateSchemaValue("input", input, schema)
}

func validateSchemaValue(path string, value any, schema map[string]any) error {
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "", "null":
		return nil
	case "object":
		mapped, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}

		properties, _ := schema["properties"].(map[string]any)
		required := schemaRequiredFields(schema["required"])
		for _, field := range required {
			if _, ok := mapped[field]; !ok {
				if path == "input" {
					return fmt.Errorf("%s is required", field)
				}
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}

		additionalAllowed := true
		if raw, ok := schema["additionalProperties"].(bool); ok {
			additionalAllowed = raw
		}
		if !additionalAllowed {
			for key := range mapped {
				if _, ok := properties[key]; !ok {
					if path == "input" {
						return fmt.Errorf("%s is not allowed", key)
					}
					return fmt.Errorf("%s.%s is not allowed", path, key)
				}
			}
		}

		for key, rawValue := range mapped {
			propertySchema, ok := properties[key].(map[string]any)
			if !ok {
				continue
			}
			nextPath := key
			if path != "input" {
				nextPath = path + "." + key
			}
			if err := validateSchemaValue(nextPath, rawValue, propertySchema); err != nil {
				return err
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		if itemSchema == nil {
			return nil
		}
		for index, item := range items {
			if err := validateSchemaValue(fmt.Sprintf("%s[%d]", path, index), item, itemSchema); err != nil {
				return err
			}
		}
	case "string":
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if enums := schemaEnumValues(schema["enum"]); len(enums) > 0 {
			for _, candidate := range enums {
				if str == candidate {
					return nil
				}
			}
			return fmt.Errorf("%s must be one of %s", path, strings.Join(enums, ", "))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "integer", "number":
		if !isSchemaNumber(value) {
			return fmt.Errorf("%s must be a number", path)
		}
	}

	return nil
}

func schemaRequiredFields(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		fields := make([]string, 0, len(typed))
		for _, item := range typed {
			if field, ok := item.(string); ok && strings.TrimSpace(field) != "" {
				fields = append(fields, field)
			}
		}
		return fields
	default:
		return nil
	}
}

func schemaEnumValues(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
		return values
	default:
		return nil
	}
}

func isSchemaNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}
