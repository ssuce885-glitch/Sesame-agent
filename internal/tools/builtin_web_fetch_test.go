package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-agent/internal/permissions"
)

func TestWebFetchToolReturnsReadableHTMLContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <title>Sesame Example</title>
    <style>body { color: red; }</style>
    <script>console.log("ignore me")</script>
  </head>
  <body>
    <main>
      <h1>Hello from Sesame</h1>
      <p>This page verifies web_fetch.</p>
    </main>
  </body>
</html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := NewRuntime(NewRegistry(), nil).ExecuteRich(context.Background(), Call{
		Name:  "web_fetch",
		Input: map[string]any{"url": server.URL + "/start"},
	}, ExecContext{
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich(web_fetch html) error = %v", err)
	}

	output, ok := result.Data.(WebFetchOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want WebFetchOutput", result.Data)
	}
	if output.FinalURL != server.URL+"/final" {
		t.Fatalf("output.FinalURL = %q, want %q", output.FinalURL, server.URL+"/final")
	}
	if output.StatusCode != http.StatusOK {
		t.Fatalf("output.StatusCode = %d, want %d", output.StatusCode, http.StatusOK)
	}
	if output.ContentKind != "html" {
		t.Fatalf("output.ContentKind = %q, want html", output.ContentKind)
	}
	if !output.Readable {
		t.Fatal("output.Readable = false, want true")
	}
	if output.Title != "Sesame Example" {
		t.Fatalf("output.Title = %q, want Sesame Example", output.Title)
	}
	if !strings.Contains(output.Content, "Hello from Sesame") {
		t.Fatalf("output.Content = %q, want heading text", output.Content)
	}
	if !strings.Contains(output.Content, "This page verifies web_fetch.") {
		t.Fatalf("output.Content = %q, want paragraph text", output.Content)
	}
	if strings.Contains(output.Content, "console.log") {
		t.Fatalf("output.Content = %q, want scripts stripped", output.Content)
	}
	if !strings.Contains(result.ModelText, "Content:") {
		t.Fatalf("result.ModelText = %q, want content block", result.ModelText)
	}
}

func TestWebFetchToolMarksBinaryResponsesUnreadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0x00, 0x01, 0x02, 0x03})
	}))
	defer server.Close()

	result, err := NewRuntime(NewRegistry(), nil).ExecuteRich(context.Background(), Call{
		Name:  "web_fetch",
		Input: map[string]any{"url": server.URL},
	}, ExecContext{
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich(web_fetch binary) error = %v", err)
	}

	output, ok := result.Data.(WebFetchOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want WebFetchOutput", result.Data)
	}
	if output.Readable {
		t.Fatal("output.Readable = true, want false")
	}
	if output.ContentKind != "unsupported" {
		t.Fatalf("output.ContentKind = %q, want unsupported", output.ContentKind)
	}
	if output.Content != "" {
		t.Fatalf("output.Content = %q, want empty", output.Content)
	}
	if !strings.Contains(result.ModelText, "not currently readable") {
		t.Fatalf("result.ModelText = %q, want unreadable notice", result.ModelText)
	}
}

func TestWebFetchToolCanTruncateLargeResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("abcdef", 20)))
	}))
	defer server.Close()

	result, err := NewRuntime(NewRegistry(), nil).ExecuteRich(context.Background(), Call{
		Name:  "web_fetch",
		Input: map[string]any{"url": server.URL, "max_bytes": 24},
	}, ExecContext{
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich(web_fetch truncate) error = %v", err)
	}

	output, ok := result.Data.(WebFetchOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want WebFetchOutput", result.Data)
	}
	if !output.Truncated {
		t.Fatal("output.Truncated = false, want true")
	}
	if output.BytesRead != 24 {
		t.Fatalf("output.BytesRead = %d, want 24", output.BytesRead)
	}
	if len(output.Content) != 24 {
		t.Fatalf("len(output.Content) = %d, want 24", len(output.Content))
	}
}

func TestWebFetchToolRejectsUnsupportedURLSchemes(t *testing.T) {
	_, err := NewRuntime(NewRegistry(), nil).ExecuteRich(context.Background(), Call{
		Name:  "web_fetch",
		Input: map[string]any{"url": "file:///etc/passwd"},
	}, ExecContext{
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("ExecuteRich(web_fetch scheme) error = %v, want http or https validation", err)
	}
}
