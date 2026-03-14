package compat

import (
	"cursor2api-go/config"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ApplyVisionInterceptor preprocesses image blocks into textual descriptions via vision API.
func ApplyVisionInterceptor(messages []AnthropicMessage, cfg *config.Config) {
	if cfg == nil || !cfg.Vision.Enabled {
		return
	}

	for idx := range messages {
		msg := &messages[idx]
		if msg.Role != "user" {
			continue
		}
		blocks := normalizeAnthropicBlocks(msg.Content)
		if len(blocks) == 0 {
			continue
		}
		var images []AnthropicContentBlock
		var textParts []string
		for _, b := range blocks {
			if b.Type == "image" && b.Source != nil {
				images = append(images, b)
			} else if b.Type == "text" && b.Text != "" {
				textParts = append(textParts, b.Text)
			}
		}
		if len(images) == 0 {
			continue
		}
		description, err := callVisionAPI(images, cfg)
		if err != nil {
			description = fmt.Sprintf("(Vision API error: %s)", err.Error())
		}
		combined := strings.Join(textParts, "\n\n")
		combined += fmt.Sprintf("\n\n[System: The user attached %d image(s). Visual analysis extracted the following context:\n%s]\n\n", len(images), description)
		msg.Content = strings.TrimSpace(combined)
	}
}

// CheckLocalOCR is a no-op now that local OCR has been removed.
func CheckLocalOCR(cfg *config.Config) error {
	return nil
}

func callVisionAPI(images []AnthropicContentBlock, cfg *config.Config) (string, error) {
	parts := []map[string]any{{
		"type": "text",
		"text": "Please describe the attached images in detail. If they contain code, UI elements, or error messages, explicitly write them out.",
	}}

	for _, img := range images {
		if img.Source == nil || img.Source.Data == "" {
			continue
		}
		url := ""
		switch img.Source.Type {
		case "base64":
			mime := img.Source.MediaType
			if mime == "" {
				mime = "image/jpeg"
			}
			url = fmt.Sprintf("data:%s;base64,%s", mime, img.Source.Data)
		case "url":
			url = img.Source.Data
		}
		if url != "" {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": url},
			})
		}
	}

	payload := map[string]any{
		"model": cfg.Vision.Model,
		"messages": []map[string]any{{
			"role":    "user",
			"content": parts,
		}},
		"max_tokens": 1500,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodPost, cfg.Vision.BaseURL, strings.NewReader(string(blob)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.Vision.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Vision.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("vision API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	choices, _ := parsed["choices"].([]any)
	if len(choices) == 0 {
		return "No description returned.", nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	content := message["content"]
	return parseVisionContent(content), nil
}

func parseVisionContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, raw := range v {
			if block, ok := raw.(map[string]any); ok {
				if text, ok := block["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return "No description returned."
}

func decodeFlexibleBase64(data string) ([]byte, error) {
	cleaned := strings.TrimSpace(data)
	cleaned = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, cleaned)

	if decoded, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	if m := len(cleaned) % 4; m != 0 {
		cleaned += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(cleaned)
}

func normalizeAnthropicBlocks(content any) []AnthropicContentBlock {
	switch v := content.(type) {
	case []AnthropicContentBlock:
		return v
	case []interface{}:
		blocks := make([]AnthropicContentBlock, 0, len(v))
		for _, raw := range v {
			if item, ok := raw.(map[string]interface{}); ok {
				block := mapToAnthropicBlock(item)
				if source, ok := item["source"].(map[string]interface{}); ok {
					block.Source = &AnthropicImageSource{
						Type:      stringValue(source["type"]),
						MediaType: stringValue(source["media_type"]),
						Data:      stringValue(source["data"]),
					}
				}
				blocks = append(blocks, block)
			}
		}
		return blocks
	default:
		return nil
	}
}
