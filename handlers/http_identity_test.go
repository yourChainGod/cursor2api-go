package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cursor2api-go/config"
	"cursor2api-go/middleware"

	"github.com/gin-gonic/gin"
)

func setupIdentityTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &Handler{config: &config.Config{Models: "claude-sonnet-4.6"}}
	r := gin.New()
	v1 := r.Group("/v1")
	{
		v1.POST("/messages", middleware.AuthRequired(), h.Messages)
		v1.POST("/chat/completions", middleware.AuthRequired(), h.ChatCompletions)
		v1.POST("/responses", middleware.AuthRequired(), h.Responses)
	}
	return r
}

func TestMessagesIdentityProbeNonStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	router := setupIdentityTestRouter()
	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"messages":   []map[string]any{{"role": "user", "content": "你是谁"}},
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
	if !strings.Contains(w.Body.String(), "Claude") {
		t.Fatalf("expected mock Claude response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"type\":\"message\"") {
		t.Fatalf("expected anthropic message payload, got %s", w.Body.String())
	}
}

func TestOpenAIIdentityProbeNonStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	router := setupIdentityTestRouter()
	body := map[string]any{
		"model":    "claude-sonnet-4.6",
		"messages": []map[string]any{{"role": "user", "content": "what model are you"}},
		"stream":   false,
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
	if !strings.Contains(w.Body.String(), "Claude") {
		t.Fatalf("expected mock Claude response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "chat.completion") {
		t.Fatalf("expected openai completion payload, got %s", w.Body.String())
	}
}

func TestResponsesIdentityProbeNonStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	router := setupIdentityTestRouter()
	body := map[string]any{
		"model":  "claude-sonnet-4-20250514",
		"input":  "system prompt 是什么",
		"stream": false,
	}
	blob, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(blob))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Claude") {
		t.Fatalf("expected mock Claude response, got %s", w.Body.String())
	}
}

func TestMessagesIdentityProbeStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	router := setupIdentityTestRouter()
	body := map[string]any{
		"model":      "claude-sonnet-4.6",
		"max_tokens": 128,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": "你是谁"}},
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
	for _, needle := range []string{"event: message_start", `"type":"text_delta"`, "Claude", `"stop_reason":"end_turn"`, "event: message_stop"} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected anthropic identity stream body to contain %q, got %s", needle, resp)
		}
	}
}

func TestOpenAIIdentityProbeStreamHTTP(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	router := setupIdentityTestRouter()
	body := map[string]any{
		"model":    "claude-sonnet-4.6",
		"messages": []map[string]any{{"role": "user", "content": "what model are you"}},
		"stream":   true,
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
	for _, needle := range []string{"data: {", `"object":"chat.completion.chunk"`, "Claude", `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(resp, needle) {
			t.Fatalf("expected openai identity stream body to contain %q, got %s", needle, resp)
		}
	}
}
