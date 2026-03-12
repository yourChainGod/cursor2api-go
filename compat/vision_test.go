package compat

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cursor2api-go/config"
)

func TestApplyVisionInterceptorWithAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "OCR extracted: hello from image"},
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
	blocks, ok := messages[0].Content.([]AnthropicContentBlock)
	if !ok {
		t.Fatalf("expected content to become []AnthropicContentBlock, got %T", messages[0].Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks after vision processing, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "please inspect" {
		t.Fatalf("expected original text block to remain, got %#v", blocks[0])
	}
	if blocks[1].Type != "text" || !strings.Contains(blocks[1].Text, "OCR extracted: hello from image") {
		t.Fatalf("expected OCR description block, got %#v", blocks[1])
	}
}

func TestProcessWithLocalOCR(t *testing.T) {
	cfg := &config.Config{Vision: config.Vision{Enabled: true, Mode: "ocr", Languages: "eng"}}
	fixturePath := filepath.Join("testdata", "hello_ocr.png")
	imageBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read OCR fixture: %v", err)
	}
	images := []AnthropicContentBlock{{
		Type:   "image",
		Source: &AnthropicImageSource{Type: "base64", MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(imageBytes)},
	}}

	text, err := processWithLocalOCR(images, cfg)
	if err != nil {
		t.Fatalf("expected local OCR to succeed, got error: %v", err)
	}
	upper := strings.ToUpper(strings.ReplaceAll(text, " ", ""))
	if !strings.Contains(upper, "HELLO") {
		t.Fatalf("expected OCR output to contain HELLO, got %q", text)
	}
}

func TestCheckLocalOCR(t *testing.T) {
	cfg := &config.Config{Vision: config.Vision{Enabled: true, Mode: "ocr", Languages: "eng"}}
	if err := CheckLocalOCR(cfg); err != nil {
		t.Fatalf("expected local OCR self-check to pass, got %v", err)
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
