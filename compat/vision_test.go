package compat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cursor2api-go/config"
)

func TestApplyVisionInterceptorWithAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "A screenshot showing code editor"},
			}},
		})
	}))
	defer server.Close()

	cfg := &config.Config{Vision: config.Vision{Enabled: true, Mode: "api", BaseURL: server.URL, APIKey: "demo", Model: "gpt-4o-mini"}}
	messages := []AnthropicMessage{{
		Role: "user",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "please inspect"},
			{Type: "image", Source: &AnthropicImageSource{Type: "base64", MediaType: "image/png", Data: "ZmFrZQ=="}},
		},
	}}

	ApplyVisionInterceptor(messages, cfg)
	text, ok := messages[0].Content.(string)
	if !ok {
		t.Fatalf("expected content to become string after vision, got %T", messages[0].Content)
	}
	if !strings.Contains(text, "A screenshot showing code editor") {
		t.Fatalf("expected vision description in output, got %q", text)
	}
	if !strings.Contains(text, "please inspect") {
		t.Fatalf("expected original text preserved, got %q", text)
	}
}

func TestCheckLocalOCRNoOp(t *testing.T) {
	cfg := &config.Config{Vision: config.Vision{Enabled: true, Mode: "ocr", Languages: "eng"}}
	if err := CheckLocalOCR(cfg); err != nil {
		t.Fatalf("CheckLocalOCR should be no-op now, got %v", err)
	}
}

func TestConvertOpenAIImageURLToAnthropicImageBlock(t *testing.T) {
	msg := OpenAIMessage{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "look at this"},
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,ZmFrZQ=="}},
		},
	}

	content := extractOpenAIContentBlocks(msg)
	blocks, ok := content.([]AnthropicContentBlock)
	if !ok {
		t.Fatalf("expected anthropic blocks, got %T", content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[1].Type != "image" || blocks[1].Source == nil || blocks[1].Source.Type != "base64" {
		t.Fatalf("expected second block to be base64 image, got %#v", blocks[1])
	}
}
