package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cursor2api-go/config"
	"cursor2api-go/middleware"
	"cursor2api-go/models"

	"github.com/gin-gonic/gin"
)

type fakeCursorExecutor struct {
	lastRequest *models.CursorRequest
	items       []interface{}
}

func (f *fakeCursorExecutor) ChatCompletionWithCursorRequest(_ context.Context, payload *models.CursorRequest) (<-chan interface{}, error) {
	f.lastRequest = payload
	ch := make(chan interface{}, len(f.items)+1)
	for _, item := range f.items {
		ch <- item
	}
	close(ch)
	return ch, nil
}

func setupTestRouterWithHandler(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	{
		v1.POST("/messages", middleware.AuthRequired("test-key"), h.Messages)
		v1.POST("/chat/completions", middleware.AuthRequired("test-key"), h.ChatCompletions)
		v1.POST("/responses", middleware.AuthRequired("test-key"), h.Responses)
	}
	return r
}

func TestOpenAIChatCompletionsToolsHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{
		"```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```",
	}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":    "claude-sonnet-4.6",
		"messages": []map[string]any{{"role": "user", "content": "read the file"}},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "Read",
				"description": "Read a file",
				"parameters":  map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}},
			},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tool_calls") || !strings.Contains(w.Body.String(), "main.go") {
		t.Fatalf("expected tool_calls response, got %s", w.Body.String())
	}
	if fake.lastRequest == nil || len(fake.lastRequest.Messages) < 3 {
		t.Fatalf("expected transformed cursor request with few-shot/tool instructions, got %#v", fake.lastRequest)
	}
}

func TestMessagesVisionHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	visionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "The image says BUILD FAILED"},
			}},
		})
	}))
	defer visionServer.Close()

	fake := &fakeCursorExecutor{items: []interface{}{"done"}}
	h := &Handler{config: &config.Config{
		Models: "claude-sonnet-4.6",
		Vision: config.Vision{Enabled: true, Mode: "api", BaseURL: visionServer.URL, APIKey: "demo", Model: "gpt-4o-mini"},
	}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "what is in this image?"},
				{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "ZmFrZQ=="}},
			},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.lastRequest == nil || len(fake.lastRequest.Messages) == 0 {
		t.Fatalf("expected cursor request to be captured")
	}
	joined := ""
	for _, msg := range fake.lastRequest.Messages {
		for _, part := range msg.Parts {
			joined += part.Text + "\n"
		}
	}
	if !strings.Contains(joined, "BUILD FAILED") {
		t.Fatalf("expected OCR/API description injected into cursor request, got %s", joined)
	}
}

func TestMessagesStreamToolUseHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{
		"First inspect.\n```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```",
	}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": "read the file"}},
		"tools": []map[string]any{{
			"name":         "Read",
			"description":  "Read a file",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")
	router.ServeHTTP(w, req)

	resp := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, resp)
	}
	for _, needle := range []string{"event: message_start", "event: content_block_start", `"type":"tool_use"`, `"partial_json":"{\"file_path\":\"main.go\"}`, `"stop_reason":"tool_use"`, "event: message_stop"} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, resp)
		}
	}
}

func TestOpenAIStreamToolCallsHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{
		"```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```",
	}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":    "claude-sonnet-4.6",
		"stream":   true,
		"messages": []map[string]any{{"role": "user", "content": "read the file"}},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "Read",
				"description": "Read a file",
				"parameters":  map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}},
			},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	router.ServeHTTP(w, req)

	resp := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, resp)
	}
	for _, needle := range []string{"data: {", `"object":"chat.completion.chunk"`, `"tool_calls":[{`, `"name":"Read"`, `"arguments":"{\"file_path\":\"main.go\"}`, `"finish_reason":"tool_calls"`, "data: [DONE]"} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, resp)
		}
	}
}

func TestResponsesStreamToolCallsHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{
		"```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```",
	}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":  "claude-sonnet-4-20250514",
		"stream": true,
		"input":  "read the file",
		"tools": []map[string]any{{
			"type":        "function",
			"name":        "Read",
			"description": "Read a file",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	router.ServeHTTP(w, req)

	resp := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, resp)
	}
	for _, needle := range []string{"data: {", `"tool_calls":[{`, `"name":"Read"`, `"finish_reason":"tool_calls"`, "data: [DONE]"} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected responses stream body to contain %q, got %s", needle, resp)
		}
	}
}

func TestMessagesVisionStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	visionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "Screenshot shows ERROR 500"},
			}},
		})
	}))
	defer visionServer.Close()

	fake := &fakeCursorExecutor{items: []interface{}{"done streaming"}}
	h := &Handler{config: &config.Config{
		Models: "claude-sonnet-4.6",
		Vision: config.Vision{Enabled: true, Mode: "api", BaseURL: visionServer.URL, APIKey: "demo", Model: "gpt-4o-mini"},
	}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"stream":     true,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "analyze this screenshot"},
				{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "ZmFrZQ=="}},
			},
		}},
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")
	router.ServeHTTP(w, req)

	resp := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, resp)
	}
	if fake.lastRequest == nil {
		t.Fatalf("expected cursor request to be captured")
	}
	joined := ""
	for _, msg := range fake.lastRequest.Messages {
		for _, part := range msg.Parts {
			joined += part.Text + "\n"
		}
	}
	if !strings.Contains(joined, "ERROR 500") {
		t.Fatalf("expected vision context in cursor request, got %s", joined)
	}
	for _, needle := range []string{"event: message_start", `"type":"text_delta"`, `"text":"done streaming"`, `"stop_reason":"end_turn"`} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, resp)
		}
	}
}
