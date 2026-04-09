package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go-agent/internal/config"
)

const (
	defaultCustomToolTimeoutSeconds = 60
	defaultCustomToolMaxOutputBytes = 64 * 1024
	maxCustomToolTimeoutSeconds     = 600
	maxCustomToolOutputBytes        = 1 << 20
)

var invalidToolNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

type ToolSpec struct {
	Name            string
	Description     string
	Path            string
	RootDir         string
	ManifestPath    string
	Scope           string
	Command         string
	Args            []string
	InputSchema     map[string]any
	OutputSchema    map[string]any
	TimeoutSeconds  int
	MaxOutputBytes  int
	ConcurrencySafe bool
	Workdir         string
}

type toolManifestFile struct {
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	Command              string         `json:"command"`
	Args                 []string       `json:"args"`
	InputSchema          map[string]any `json:"input_schema"`
	InputSchemaCamel     map[string]any `json:"inputSchema"`
	OutputSchema         map[string]any `json:"output_schema"`
	OutputSchemaCamel    map[string]any `json:"outputSchema"`
	TimeoutSeconds       int            `json:"timeout_seconds"`
	TimeoutSecondsCamel  int            `json:"timeoutSeconds"`
	MaxOutputBytes       int            `json:"max_output_bytes"`
	MaxOutputBytesCamel  int            `json:"maxOutputBytes"`
	ConcurrencySafe      *bool          `json:"concurrency_safe"`
	ConcurrencySafeCamel *bool          `json:"concurrencySafe"`
	Workdir              string         `json:"workdir"`
}

func DiscoverToolSpecs(globalRoot, workspaceRoot string) ([]ToolSpec, error) {
	paths, err := config.ResolvePaths(workspaceRoot, "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(globalRoot) != "" {
		paths.GlobalRoot = globalRoot
		paths.GlobalToolsDir = filepath.Join(globalRoot, "tools")
	}

	globalTools, err := readToolSpecsDir(paths.GlobalToolsDir, "global")
	if err != nil {
		return nil, err
	}
	workspaceTools, err := readToolSpecsDir(paths.WorkspaceToolsDir, "workspace")
	if err != nil {
		return nil, err
	}

	toolMap := make(map[string]ToolSpec, len(globalTools)+len(workspaceTools))
	for _, spec := range globalTools {
		toolMap[strings.ToLower(spec.Name)] = spec
	}
	for _, spec := range workspaceTools {
		toolMap[strings.ToLower(spec.Name)] = spec
	}

	specs := make([]ToolSpec, 0, len(toolMap))
	for _, spec := range toolMap {
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		left := strings.ToLower(specs[i].Name)
		right := strings.ToLower(specs[j].Name)
		if left == right {
			return specs[i].Name < specs[j].Name
		}
		return left < right
	})
	return specs, nil
}

func readToolSpecsDir(root string, scope string) ([]ToolSpec, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	specs := make([]ToolSpec, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(root, name)
		if entry.IsDir() {
			spec, ok := loadToolSpecFromManifest(filepath.Join(entryPath, "tool.json"), entryPath, entryPath, scope, name)
			if ok {
				specs = append(specs, spec)
			}
			continue
		}

		if filepath.Ext(name) != ".json" {
			continue
		}
		spec, ok := loadToolSpecFromManifest(entryPath, filepath.Dir(entryPath), entryPath, scope, strings.TrimSuffix(name, filepath.Ext(name)))
		if ok {
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

func loadToolSpecFromManifest(manifestPath, toolRoot, displayPath, scope, defaultName string) (ToolSpec, bool) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ToolSpec{}, false
	}

	var manifest toolManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ToolSpec{}, false
	}

	name := normalizeToolName(firstNonEmptyToolString(manifest.Name, defaultName))
	command := strings.TrimSpace(manifest.Command)
	if name == "" || command == "" {
		return ToolSpec{}, false
	}

	inputSchema := firstNonNilMap(manifest.InputSchema, manifest.InputSchemaCamel)
	if inputSchema == nil {
		inputSchema = emptyToolInputSchema()
	}
	outputSchema := firstNonNilMap(manifest.OutputSchema, manifest.OutputSchemaCamel)

	timeoutSeconds := firstPositiveToolInt(manifest.TimeoutSeconds, manifest.TimeoutSecondsCamel)
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultCustomToolTimeoutSeconds
	}
	if timeoutSeconds > maxCustomToolTimeoutSeconds {
		timeoutSeconds = maxCustomToolTimeoutSeconds
	}

	maxOutputBytes := firstPositiveToolInt(manifest.MaxOutputBytes, manifest.MaxOutputBytesCamel)
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultCustomToolMaxOutputBytes
	}
	if maxOutputBytes > maxCustomToolOutputBytes {
		maxOutputBytes = maxCustomToolOutputBytes
	}

	concurrencySafe := false
	switch {
	case manifest.ConcurrencySafe != nil:
		concurrencySafe = *manifest.ConcurrencySafe
	case manifest.ConcurrencySafeCamel != nil:
		concurrencySafe = *manifest.ConcurrencySafeCamel
	}

	return ToolSpec{
		Name:            name,
		Description:     strings.TrimSpace(manifest.Description),
		Path:            displayPath,
		RootDir:         toolRoot,
		ManifestPath:    manifestPath,
		Scope:           scope,
		Command:         command,
		Args:            append([]string(nil), manifest.Args...),
		InputSchema:     cloneToolSchemaMap(inputSchema),
		OutputSchema:    cloneToolSchemaMap(outputSchema),
		TimeoutSeconds:  timeoutSeconds,
		MaxOutputBytes:  maxOutputBytes,
		ConcurrencySafe: concurrencySafe,
		Workdir:         strings.TrimSpace(manifest.Workdir),
	}, true
}

func emptyToolInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func normalizeToolName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = invalidToolNameChars.ReplaceAllString(name, "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_-")
	if name == "" {
		return ""
	}
	if len(name) > 64 {
		name = strings.Trim(name[:64], "_-")
	}
	if name == "" {
		return ""
	}
	first := name[0]
	if (first < 'a' || first > 'z') && (first < '0' || first > '9') {
		name = "tool_" + name
		if len(name) > 64 {
			name = strings.Trim(name[:64], "_-")
		}
	}
	return name
}

func firstNonEmptyToolString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositiveToolInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonNilMap(values ...map[string]any) map[string]any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func cloneToolSchemaMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneToolSchemaValue(value)
	}
	return out
}

func cloneToolSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneToolSchemaMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneToolSchemaValue(item)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return value
	}
}
