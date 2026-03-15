// Copyright (c) 2025-2026 libaxuan
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package models

import "strings"

// ModelConfig 模型配置结构
type ModelConfig struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	MaxTokens     int    `json:"max_tokens"`
	ContextWindow int    `json:"context_window"`
	CursorModel   string `json:"cursor_model"` // Cursor API 使用的实际模型名
}

// GetModelConfigs 获取所有模型配置
func GetModelConfigs() map[string]ModelConfig {
	return map[string]ModelConfig{
		"claude-sonnet-4.6": {
			ID:            "claude-sonnet-4.6",
			Provider:      "Anthropic",
			MaxTokens:     200000,
			ContextWindow: 200000,
			CursorModel:   "anthropic/claude-sonnet-4.6",
		},
		"claude-sonnet-4-5-20250929": {
			ID:            "claude-sonnet-4-5-20250929",
			Provider:      "Anthropic",
			MaxTokens:     200000,
			ContextWindow: 200000,
			CursorModel:   "anthropic/claude-sonnet-4.6",
		},
		"claude-sonnet-4-20250514": {
			ID:            "claude-sonnet-4-20250514",
			Provider:      "Anthropic",
			MaxTokens:     200000,
			ContextWindow: 200000,
			CursorModel:   "anthropic/claude-sonnet-4.6",
		},
		"claude-3-5-sonnet-20241022": {
			ID:            "claude-3-5-sonnet-20241022",
			Provider:      "Anthropic",
			MaxTokens:     200000,
			ContextWindow: 200000,
			CursorModel:   "anthropic/claude-sonnet-4.6",
		},
	}
}

// GetModelConfig 获取指定模型的配置。
// 支持 `-thinking` 后缀：自动剥离后查找基础模型。
func GetModelConfig(modelID string) (ModelConfig, bool) {
	configs := GetModelConfigs()
	if config, exists := configs[modelID]; exists {
		return config, exists
	}
	// Strip -thinking suffix and try again
	base := strings.TrimSuffix(modelID, "-thinking")
	if base != modelID {
		if config, exists := configs[base]; exists {
			return config, exists
		}
	}
	return ModelConfig{}, false
}

// IsThinkingModel returns true if the model ID ends with "-thinking".
func IsThinkingModel(modelID string) bool {
	return strings.HasSuffix(modelID, "-thinking")
}

// BaseModelID strips the "-thinking" suffix if present.
func BaseModelID(modelID string) string {
	return strings.TrimSuffix(modelID, "-thinking")
}

// ExpandModelsWithThinking takes a list of model IDs and appends
// "-thinking" variants for each, deduplicating.
func ExpandModelsWithThinking(models []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(models)*2)
	for _, m := range models {
		if !seen[m] {
			result = append(result, m)
			seen[m] = true
		}
		thinking := m + "-thinking"
		if !seen[thinking] {
			result = append(result, thinking)
			seen[thinking] = true
		}
	}
	return result
}

// GetCursorModel 获取Cursor API使用的模型名称
func GetCursorModel(modelID string) string {
	if config, exists := GetModelConfig(modelID); exists {
		if config.CursorModel != "" {
			return config.CursorModel
		}
	}
	// 如果没有配置映射，返回原始模型名
	return modelID
}

// ResolveCursorModel resolves a requested API model to a concrete Cursor backend model,
// falling back to the configured default model when needed.
func ResolveCursorModel(modelID, fallbackModelID string) string {
	if resolved := GetCursorModel(modelID); resolved != "" && resolved != modelID {
		return resolved
	}
	if config, exists := GetModelConfig(modelID); exists && config.CursorModel != "" {
		return config.CursorModel
	}
	if fallbackModelID != "" {
		if config, exists := GetModelConfig(fallbackModelID); exists && config.CursorModel != "" {
			return config.CursorModel
		}
		if fallbackModelID != modelID {
			if resolved := GetCursorModel(fallbackModelID); resolved != "" {
				return resolved
			}
		}
	}
	return GetCursorModel(modelID)
}

// GetMaxTokensForModel 获取指定模型的最大token数
func GetMaxTokensForModel(modelID string) int {
	if config, exists := GetModelConfig(modelID); exists {
		return config.MaxTokens
	}
	// 默认返回4096
	return 4096
}

// GetContextWindowForModel 获取指定模型的上下文窗口大小
func GetContextWindowForModel(modelID string) int {
	if config, exists := GetModelConfig(modelID); exists {
		return config.ContextWindow
	}
	// 默认返回128000
	return 128000
}

// ValidateMaxTokens 验证并调整max_tokens参数
func ValidateMaxTokens(modelID string, requestedMaxTokens *int) *int {
	modelMaxTokens := GetMaxTokensForModel(modelID)

	// 如果没有指定max_tokens，使用模型默认值
	if requestedMaxTokens == nil {
		return &modelMaxTokens
	}

	// 如果请求的max_tokens超过模型限制，使用模型最大值
	if *requestedMaxTokens > modelMaxTokens {
		return &modelMaxTokens
	}

	// 如果请求的max_tokens小于等于0，使用模型默认值
	if *requestedMaxTokens <= 0 {
		return &modelMaxTokens
	}

	return requestedMaxTokens
}
