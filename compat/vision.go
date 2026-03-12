package compat

import (
	"cursor2api-go/config"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/otiai10/gosseract/v2"
)

var defaultOCRLanguages = []string{"eng", "chi_sim"}

//go:embed testdata/hello_ocr.png
var localOCRSelfCheckPNG []byte

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
	return processWithLocalOCR(images, cfg)
}

// CheckLocalOCR performs a startup/self-check of the local OCR backend when
// VISION_ENABLED=true and VISION_MODE=ocr. It is intentionally strict so the
// service fails early instead of silently accepting a broken OCR environment.
func CheckLocalOCR(cfg *config.Config) error {
	if cfg == nil || !cfg.Vision.Enabled || !strings.EqualFold(cfg.Vision.Mode, "ocr") {
		return nil
	}
	if len(localOCRSelfCheckPNG) == 0 {
		return fmt.Errorf("local OCR self-check fixture is empty")
	}
	text, err := processWithLocalOCR([]AnthropicContentBlock{{
		Type:   "image",
		Source: &AnthropicImageSource{Type: "base64", MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(localOCRSelfCheckPNG)},
	}}, cfg)
	if err != nil {
		return fmt.Errorf("local OCR self-check failed: %w", err)
	}
	normalized := strings.ToUpper(strings.ReplaceAll(text, " ", ""))
	if !strings.Contains(normalized, "HELLO") {
		return fmt.Errorf("local OCR self-check failed: unexpected OCR output %q", text)
	}
	return nil
}

func processWithLocalOCR(images []AnthropicContentBlock, cfg *config.Config) (string, error) {
	client := gosseract.NewClient()
	defer client.Close()

	if err := client.DisableOutput(); err != nil {
		return "", fmt.Errorf("disable tesseract output: %w", err)
	}

	langs := resolveOCRLanguages(cfg)
	if err := client.SetLanguage(langs...); err != nil {
		return "", fmt.Errorf("set tesseract languages: %w", err)
	}

	parts := make([]string, 0, len(images))
	for i, img := range images {
		imageBytes, err := loadOCRImageBytes(img)
		if err != nil {
			parts = append(parts, fmt.Sprintf("--- Image %d ---\n(Failed to load image for OCR: %s)", i+1, err.Error()))
			continue
		}
		if err := client.SetImageFromBytes(imageBytes); err != nil {
			parts = append(parts, fmt.Sprintf("--- Image %d ---\n(Failed to prepare image for OCR: %s)", i+1, err.Error()))
			continue
		}
		text, err := client.Text()
		if err != nil {
			parts = append(parts, fmt.Sprintf("--- Image %d ---\n(Failed to parse image with gosseract: %s)", i+1, err.Error()))
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			text = "(No text detected in this image)"
		}
		parts = append(parts, fmt.Sprintf("--- Image %d OCR Text ---\n%s", i+1, text))
	}

	combined := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if combined == "" {
		return "No text detected in the attached image(s).", nil
	}
	return combined, nil
}

func resolveOCRLanguages(cfg *config.Config) []string {
	if cfg == nil || strings.TrimSpace(cfg.Vision.Languages) == "" {
		return defaultOCRLanguages
	}
	items := strings.Split(cfg.Vision.Languages, ",")
	langs := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			langs = append(langs, trimmed)
		}
	}
	if len(langs) == 0 {
		return defaultOCRLanguages
	}
	return langs
}

func loadOCRImageBytes(img AnthropicContentBlock) ([]byte, error) {
	if img.Source == nil || img.Source.Data == "" {
		return nil, fmt.Errorf("image source is empty")
	}

	switch img.Source.Type {
	case "base64":
		data := img.Source.Data
		if strings.HasPrefix(data, "data:") {
			_, data = parseDataURL(data)
		}
		decoded, err := decodeFlexibleBase64(data)
		if err != nil {
			return nil, fmt.Errorf("decode base64 image: %w", err)
		}
		return decoded, nil
	case "url":
		if strings.HasPrefix(img.Source.Data, "data:") {
			_, data := parseDataURL(img.Source.Data)
			decoded, err := decodeFlexibleBase64(data)
			if err != nil {
				return nil, fmt.Errorf("decode data-url image: %w", err)
			}
			return decoded, nil
		}
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(img.Source.Data)
		if err != nil {
			return nil, fmt.Errorf("fetch remote image: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return nil, fmt.Errorf("fetch remote image returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read remote image body: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported image source type: %s", img.Source.Type)
	}
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
