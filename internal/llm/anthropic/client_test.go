package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/hollis-labs/loom/internal/llm"
	"github.com/hollis-labs/loom/internal/llm/anthropic"
)

// fakeAnthropicServer stands in for the real Anthropic Messages API so
// Client can be unit-tested without live network calls or a real API key.
func fakeAnthropicServer(t *testing.T, respBody string, capture *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			capture.path = r.URL.Path
			capture.method = r.Method
			_ = json.NewDecoder(r.Body).Decode(&capture.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
}

type capturedRequest struct {
	path   string
	method string
	body   map[string]any
}

func TestClientGenerateReturnsResponseText(t *testing.T) {
	var captured capturedRequest
	server := fakeAnthropicServer(t, `{
		"id": "msg_test",
		"type": "message",
		"role": "assistant",
		"model": "claude-test",
		"content": [{"type": "text", "text": "# Runtime\n\nGenerated body."}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`, &captured)
	defer server.Close()

	client, err := anthropic.New("test-key", "claude-test", option.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	text, err := client.Generate(context.Background(), llm.Prompt{Title: "Runtime", Material: "notes"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if text != "# Runtime\n\nGenerated body." {
		t.Fatalf("text = %q", text)
	}
	if captured.method != http.MethodPost || !strings.Contains(captured.path, "v1/messages") {
		t.Fatalf("request = %s %s, want POST .../v1/messages", captured.method, captured.path)
	}
	if captured.body["model"] != "claude-test" {
		t.Fatalf("request model = %v, want claude-test", captured.body["model"])
	}
	system, _ := captured.body["system"].([]any)
	if len(system) == 0 {
		t.Fatalf("request missing system prompt: %+v", captured.body)
	}
}

func TestClientRequiresModel(t *testing.T) {
	if _, err := anthropic.New("test-key", ""); err == nil {
		t.Fatal("New: want error for empty model")
	}
}

func TestClientGenerateEmptyResponseErrors(t *testing.T) {
	server := fakeAnthropicServer(t, `{
		"id": "msg_empty",
		"type": "message",
		"role": "assistant",
		"model": "claude-test",
		"content": [],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 1, "output_tokens": 0}
	}`, nil)
	defer server.Close()

	client, err := anthropic.New("test-key", "claude-test", option.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Generate(context.Background(), llm.Prompt{Title: "Empty"}); err == nil {
		t.Fatal("Generate: want error for empty response content")
	}
}

func TestClientGenerateSurfacesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
	}))
	defer server.Close()

	client, err := anthropic.New("test-key", "claude-test", option.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Generate(context.Background(), llm.Prompt{Title: "Broken"}); err == nil {
		t.Fatal("Generate: want error for 500 response")
	}
}
