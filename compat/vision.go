package compat

import (
	"bytes"
	"cursor2api-go/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ApplyVisionInterceptor preprocesses image blocks into textual descriptions/OCR output.
func ApplyVisionInterceptor(messages []AnthropicMessage, cfg *config.Config) {
	if cfg == nil || !cfg.Vision.Enabled {
		return
	}

	for i := range messages {
		blocks := normalizeAnthropicBlocks(messages[i].Content)
		if len(blocks) == 0 {
			continue
		}

		nonImages := make([]AnthropicContentBlock, 0, len(blocks))
		images := make([]AnthropicContentBlock, 0)
		for _, block := range blocks {
			if block.Type == "image" && block.Source != nil && block.Source.Data != "" {
				images = append(images, block)
			} else {
				nonImages = append(nonImages, block)
			}
		}
		if len(images) == 0 {
			continue
		}

		description, err := processVision(images, cfg)
		if err != nil {
			nonImages = append(nonImages, AnthropicContentBlock{
				Type: "text",
				Text: fmt.Sprintf("\n\n[System: The user attached %d image(s), but the Vision interceptor failed to process them. Error: %s]\n\n", len(images), err.Error()),
			})
		} else {
			nonImages = append(nonImages, AnthropicContentBlock{
				Type: "text",
				Text: fmt.Sprintf("\n\n[System: The user attached %d image(s). Visual analysis/OCR extracted the following context:\n%s]\n\n", len(images), description),
			})
		}
		messages[i].Content = nonImages
	}
}

func processVision(images []AnthropicContentBlock, cfg *config.Config) (string, error) {
	if strings.EqualFold(cfg.Vision.Mode, "api") {
		return callVisionAPI(images, cfg)
	}
	return processWithLocalOCR(images)
}

func processWithLocalOCR(images []AnthropicContentBlock) (string, error) {
	payload := map[string]any{
		"images": images,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("node", filepath.Join("jscode", "vision_ocr.mjs"))
	cmd.Stdin = bytes.NewReader(blob)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("local OCR failed: %s", strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("local OCR failed: %w", err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return "", fmt.Errorf("invalid OCR output: %w", err)
	}
	if strings.TrimSpace(result.Error) != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	if strings.TrimSpace(result.Text) == "" {
		return "No text detected in the attached image(s).", nil
	}
	return result.Text, nil
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
	req, err := http.NewRequest(http.MethodPost, cfg.Vision.BaseURL, bytes.NewReader(blob))
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
