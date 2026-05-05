package contracts

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestToolPolicyRuleMarshalAlwaysObjectAndBoolCompatibility(t *testing.T) {
	jsonRaw, err := json.Marshal(map[string]ToolPolicyRule{
		"shell": {Allowed: boolPtr(false)},
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if strings.Contains(string(jsonRaw), `"shell":false`) || !strings.Contains(string(jsonRaw), `"allowed":false`) {
		t.Fatalf("expected object JSON form, got %s", jsonRaw)
	}

	yamlRaw, err := yaml.Marshal(map[string]ToolPolicyRule{
		"shell": {Allowed: boolPtr(false)},
	})
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if strings.Contains(string(yamlRaw), "shell: false") || !strings.Contains(string(yamlRaw), "allowed: false") {
		t.Fatalf("expected object YAML form, got %s", yamlRaw)
	}

	var decodedJSON map[string]ToolPolicyRule
	if err := json.Unmarshal([]byte(`{"shell":false}`), &decodedJSON); err != nil {
		t.Fatalf("UnmarshalJSON bool form: %v", err)
	}
	if allowed := decodedJSON["shell"].Allowed; allowed == nil || *allowed {
		t.Fatalf("expected bool JSON compatibility, got %+v", decodedJSON["shell"])
	}

	var decodedYAML map[string]ToolPolicyRule
	if err := yaml.Unmarshal([]byte("shell: false\n"), &decodedYAML); err != nil {
		t.Fatalf("UnmarshalYAML bool form: %v", err)
	}
	if allowed := decodedYAML["shell"].Allowed; allowed == nil || *allowed {
		t.Fatalf("expected bool YAML compatibility, got %+v", decodedYAML["shell"])
	}
}

func boolPtr(value bool) *bool {
	return &value
}
