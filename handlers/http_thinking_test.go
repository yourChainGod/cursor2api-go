package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cursor2api-go/config"
)

func TestMessagesThinkingNonStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{"<thinking>Plan first</thinking>Final answer"}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"messages":   []map[string]any{{"role": "user", "content": "help me"}},
		"thinking":   map[string]any{"type": "enabled", "budget_tokens": 1024},
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
	if !strings.Contains(resp, `"type":"thinking"`) || !strings.Contains(resp, `"thinking":"Plan first"`) || !strings.Contains(resp, `"text":"Final answer"`) {
		t.Fatalf("expected thinking + text in response, got %s", resp)
	}
}

func TestMessagesThinkingStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	fake := &fakeCursorExecutor{items: []interface{}{"<thinking>Plan first</thinking>Final answer"}}
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}, cursorService: fake}
	router := setupTestRouterWithHandler(h)

	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": "help me"}},
		"thinking":   map[string]any{"type": "enabled", "budget_tokens": 1024},
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
	for _, needle := range []string{`"type":"thinking"`, `"type":"thinking_delta"`, `"thinking":"Plan first"`, `"type":"text_delta"`, `Final answer`} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected stream body to contain %q, got %s", needle, resp)
		}
	}
}
