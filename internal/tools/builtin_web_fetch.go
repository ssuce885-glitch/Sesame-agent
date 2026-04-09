package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultWebFetchTimeoutSeconds = 20
	maxWebFetchTimeoutSeconds     = 60
	defaultWebFetchMaxBytes       = 16 * 1024
	maxWebFetchMaxBytes           = 64 * 1024
	webFetchUserAgent             = "Sesame/0.1"
)

var (
	webFetchTitlePattern       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	webFetchCommentPattern     = regexp.MustCompile(`(?is)<!--.*?-->`)
	webFetchScriptStylePattern = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<noscript[^>]*>.*?</noscript>`)
	webFetchBlockTagPattern    = regexp.MustCompile(`(?i)</?(address|article|aside|blockquote|br|dd|div|dl|dt|fieldset|figcaption|figure|footer|form|h[1-6]|header|hr|li|main|nav|ol|p|pre|section|table|td|th|tr|ul)[^>]*>`)
	webFetchTagPattern         = regexp.MustCompile(`(?s)<[^>]+>`)
	webFetchInlineSpacePattern = regexp.MustCompile(`[ \t\f\v]+`)
	webFetchBlankLinePattern   = regexp.MustCompile(`\n{3,}`)
)

type webFetchTool struct{}

type WebFetchInput struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxBytes       int    `json:"max_bytes"`
}

type WebFetchOutput struct {
	URL         string `json:"url"`
	FinalURL    string `json:"final_url"`
	StatusCode  int    `json:"status_code"`
	Status      string `json:"status"`
	ContentType string `json:"content_type"`
	ContentKind string `json:"content_kind"`
	Readable    bool   `json:"readable"`
	Title       string `json:"title,omitempty"`
	Content     string `json:"content"`
	BytesRead   int    `json:"bytes_read"`
	Truncated   bool   `json:"truncated"`
}

func (webFetchTool) Definition() Definition {
	return Definition{
		Name:           "web_fetch",
		Description:    "Fetch a web page or text resource over HTTP(S) and return readable text content. HTML responses are converted to plain text.",
		MaxInlineBytes: defaultWebFetchMaxBytes,
		InputSchema: objectSchema(map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The http:// or https:// URL to fetch.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional request timeout in seconds.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Optional maximum number of response bytes to read before truncating.",
			},
		}, "url"),
		OutputSchema: objectSchema(map[string]any{
			"url":          map[string]any{"type": "string"},
			"final_url":    map[string]any{"type": "string"},
			"status_code":  map[string]any{"type": "integer"},
			"status":       map[string]any{"type": "string"},
			"content_type": map[string]any{"type": "string"},
			"content_kind": map[string]any{"type": "string"},
			"readable":     map[string]any{"type": "boolean"},
			"title":        map[string]any{"type": "string"},
			"content":      map[string]any{"type": "string"},
			"bytes_read":   map[string]any{"type": "integer"},
			"truncated":    map[string]any{"type": "boolean"},
		}, "url", "final_url", "status_code", "status", "content_type", "content_kind", "readable", "content", "bytes_read", "truncated"),
	}
}

func (webFetchTool) IsConcurrencySafe() bool { return true }

func (webFetchTool) Decode(call Call) (DecodedCall, error) {
	rawURL := strings.TrimSpace(call.StringInput("url"))
	if rawURL == "" {
		return DecodedCall{}, fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return DecodedCall{}, fmt.Errorf("url must be a valid http or https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return DecodedCall{}, fmt.Errorf("url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return DecodedCall{}, fmt.Errorf("url host is required")
	}

	timeoutSeconds, err := decodeShellPositiveInt(call.Input["timeout_seconds"], defaultWebFetchTimeoutSeconds)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("timeout_seconds %w", err)
	}
	if timeoutSeconds > maxWebFetchTimeoutSeconds {
		return DecodedCall{}, fmt.Errorf("timeout_seconds exceeds max allowed (%d)", maxWebFetchTimeoutSeconds)
	}

	maxBytes, err := decodeShellPositiveInt(call.Input["max_bytes"], defaultWebFetchMaxBytes)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("max_bytes %w", err)
	}
	if maxBytes > maxWebFetchMaxBytes {
		return DecodedCall{}, fmt.Errorf("max_bytes exceeds max allowed (%d)", maxWebFetchMaxBytes)
	}

	normalizedURL := parsed.String()
	normalized := Call{
		ID:   call.ID,
		Name: call.Name,
		Input: map[string]any{
			"url":             normalizedURL,
			"timeout_seconds": timeoutSeconds,
			"max_bytes":       maxBytes,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: WebFetchInput{
			URL:            normalizedURL,
			TimeoutSeconds: timeoutSeconds,
			MaxBytes:       maxBytes,
		},
	}, nil
}

func (t webFetchTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (webFetchTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, _ ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(WebFetchInput)
	if input.URL == "" {
		return ToolExecutionResult{}, fmt.Errorf("url is required")
	}
	if input.TimeoutSeconds <= 0 {
		input.TimeoutSeconds = defaultWebFetchTimeoutSeconds
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = defaultWebFetchMaxBytes
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, input.URL, nil)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", webFetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,text/markdown,application/xml,text/xml,*/*;q=0.8")

	client := &http.Client{
		Timeout: time.Duration(input.TimeoutSeconds) * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("fetch %s: %w", input.URL, err)
	}
	defer resp.Body.Close()

	body, truncated, err := readWebFetchBody(resp.Body, input.MaxBytes)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("read response body: %w", err)
	}

	finalURL := input.URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	mediaType := contentType
	if parsedMediaType, _, err := mime.ParseMediaType(contentType); err == nil && parsedMediaType != "" {
		mediaType = parsedMediaType
	}
	if strings.TrimSpace(mediaType) == "" {
		mediaType = http.DetectContentType(body)
		contentType = mediaType
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = mediaType
	}

	contentKind, readable := classifyWebFetchContent(mediaType, body)
	title := ""
	content := ""
	switch contentKind {
	case "html":
		title, content = extractWebFetchHTML(body)
	case "json":
		content = formatWebFetchJSON(body)
	case "text", "xml":
		content = normalizeWebFetchText(body)
	}
	if !readable {
		content = ""
	}

	output := WebFetchOutput{
		URL:         input.URL,
		FinalURL:    finalURL,
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		ContentType: contentType,
		ContentKind: contentKind,
		Readable:    readable,
		Title:       title,
		Content:     content,
		BytesRead:   len(body),
		Truncated:   truncated,
	}
	modelText := renderWebFetchModelText(output)
	return ToolExecutionResult{
		Result: Result{
			Text:      modelText,
			ModelText: modelText,
		},
		Data:        output,
		PreviewText: renderWebFetchPreview(output),
		Metadata: map[string]any{
			"status_code":  output.StatusCode,
			"content_type": output.ContentType,
			"content_kind": output.ContentKind,
			"readable":     output.Readable,
			"final_url":    output.FinalURL,
			"truncated":    output.Truncated,
		},
	}, nil
}

func (webFetchTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func readWebFetchBody(body io.Reader, maxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultWebFetchMaxBytes
	}
	limited := io.LimitReader(body, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return data, truncated, nil
}

func classifyWebFetchContent(contentType string, body []byte) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(normalized, "html"):
		return "html", true
	case strings.Contains(normalized, "json"):
		return "json", true
	case strings.Contains(normalized, "xml"):
		return "xml", true
	case strings.HasPrefix(normalized, "text/"):
		return "text", true
	case strings.Contains(normalized, "javascript"), strings.Contains(normalized, "ecmascript"):
		return "text", true
	}

	if utf8.Valid(body) && !looksBinaryContent(body) {
		return "text", true
	}
	return "unsupported", false
}

func looksBinaryContent(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	sample := body
	if len(sample) > 512 {
		sample = sample[:512]
	}
	controlBytes := 0
	for _, b := range sample {
		if b == 0 {
			return true
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) {
			controlBytes++
		}
	}
	return float64(controlBytes)/float64(len(sample)) > 0.1
}

func extractWebFetchHTML(body []byte) (string, string) {
	raw := normalizeWebFetchText(body)
	titleMatch := webFetchTitlePattern.FindStringSubmatch(raw)
	title := ""
	if len(titleMatch) > 1 {
		title = normalizeWebFetchLine(titleMatch[1])
	}

	cleaned := webFetchCommentPattern.ReplaceAllString(raw, "\n")
	cleaned = webFetchScriptStylePattern.ReplaceAllString(cleaned, "\n")
	cleaned = webFetchBlockTagPattern.ReplaceAllString(cleaned, "\n")
	cleaned = webFetchTagPattern.ReplaceAllString(cleaned, " ")
	cleaned = html.UnescapeString(cleaned)

	lines := strings.Split(cleaned, "\n")
	filtered := make([]string, 0, len(lines))
	lastBlank := true
	for _, line := range lines {
		line = normalizeWebFetchLine(line)
		if line == "" {
			if !lastBlank && len(filtered) > 0 {
				filtered = append(filtered, "")
			}
			lastBlank = true
			continue
		}
		filtered = append(filtered, line)
		lastBlank = false
	}

	text := strings.TrimSpace(strings.Join(filtered, "\n"))
	text = webFetchBlankLinePattern.ReplaceAllString(text, "\n\n")
	return title, text
}

func formatWebFetchJSON(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, trimmed, "", "  "); err == nil {
		return pretty.String()
	}
	return normalizeWebFetchText(trimmed)
}

func normalizeWebFetchText(body []byte) string {
	text := string(body)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func normalizeWebFetchLine(line string) string {
	line = html.UnescapeString(line)
	line = strings.TrimSpace(line)
	line = webFetchInlineSpacePattern.ReplaceAllString(line, " ")
	return line
}

func renderWebFetchModelText(output WebFetchOutput) string {
	lines := []string{
		fmt.Sprintf("Fetched %s", output.URL),
		fmt.Sprintf("Final URL: %s", output.FinalURL),
		fmt.Sprintf("Status: %s", output.Status),
		fmt.Sprintf("Content-Type: %s", output.ContentType),
	}
	if output.Title != "" {
		lines = append(lines, "Title: "+output.Title)
	}
	if output.Truncated {
		lines = append(lines, fmt.Sprintf("Body truncated after %d bytes.", output.BytesRead))
	}
	if !output.Readable {
		lines = append(lines, "This response is not currently readable as text by web_fetch.")
		return strings.Join(lines, "\n")
	}
	if strings.TrimSpace(output.Content) == "" {
		lines = append(lines, "Content is empty.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Content:", output.Content)
	return strings.Join(lines, "\n")
}

func renderWebFetchPreview(output WebFetchOutput) string {
	parts := []string{
		fmt.Sprintf("Fetched %s (%s)", output.FinalURL, output.Status),
	}
	if output.Title != "" {
		parts = append(parts, fmt.Sprintf("title=%q", output.Title))
	}
	if output.Truncated {
		parts = append(parts, "truncated")
	}
	if !output.Readable {
		parts = append(parts, "unreadable")
	}
	return strings.Join(parts, ", ")
}
