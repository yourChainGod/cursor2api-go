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

package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Config 应用程序配置结构
type Config struct {
	// 服务器配置
	Port  int  `json:"port"`
	Debug bool `json:"debug"`

	// API配置
	APIKey             string `json:"api_key"`
	Models             string `json:"models"`
	SystemPromptInject string `json:"system_prompt_inject"`
	Timeout            int    `json:"timeout"`
	MaxInputLength     int    `json:"max_input_length"`

	// 网络配置
	Proxy string `json:"proxy"`

	// 请求头 / 指纹相关配置
	FP     FP     `json:"fp"`
	Vision Vision `json:"vision"`
}

// FP 指纹配置结构
type FP struct {
	UserAgent string `json:"userAgent"`
}

// Vision 视觉/OCR配置
type Vision struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Mode      string `json:"mode" yaml:"mode"`
	BaseURL   string `json:"base_url" yaml:"base_url"`
	APIKey    string `json:"api_key" yaml:"api_key"`
	Model     string `json:"model" yaml:"model"`
	Languages string `json:"languages" yaml:"languages"`
}

type yamlConfig struct {
	Port               int    `yaml:"port"`
	Debug              bool   `yaml:"debug"`
	APIKey             string `yaml:"api_key"`
	Models             string `yaml:"models"`
	CursorModel        string `yaml:"cursor_model"` // backward-compatible alias
	SystemPromptInject string `yaml:"system_prompt_inject"`
	Timeout            int    `yaml:"timeout"`
	MaxInputLength     int    `yaml:"max_input_length"`
	Proxy              string `yaml:"proxy"`
	Fingerprint        struct {
		UserAgent string `yaml:"user_agent"`
	} `yaml:"fingerprint"`
	Vision Vision `yaml:"vision"`
}

// generateAPIKey creates a random sk- prefixed 64-character key.
func generateAPIKey() string {
	b := make([]byte, 31)
	if _, err := rand.Read(b); err != nil {
		return "sk-change-me-please-00000000000000000000000000000000000000"
	}
	return "sk-" + hex.EncodeToString(b) // 3 + 62 = 65 chars, trim to 64
}

// ensureDefaultConfig creates a config.yaml with sane defaults and a random
// API key when neither config.yaml, config.yml, nor .env exist.
func ensureDefaultConfig() {
	for _, path := range []string{"config.yaml", "config.yml", ".env"} {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	// Also skip if API_KEY env var is already set
	if os.Getenv("API_KEY") != "" {
		return
	}

	key := generateAPIKey()
	content := fmt.Sprintf(`# cursor2api-go default configuration (auto-generated)
port: 8002
debug: false
api_key: "%s"
models: "claude-sonnet-4.6,claude-sonnet-4-5-20250929,claude-sonnet-4-20250514,claude-3-5-sonnet-20241022"
timeout: 120
max_input_length: 200000

# vision:
#   enabled: false
#   mode: api
#   base_url: "https://api.openai.com/v1/chat/completions"
#   api_key: ""
#   model: "gpt-4o-mini"
`, key)

	if err := os.WriteFile("config.yaml", []byte(content), 0600); err != nil {
		logrus.Warnf("Failed to write default config.yaml: %v", err)
		return
	}
	logrus.Infof("Generated default config.yaml with API key: %s", key)
}

// LoadConfig 加载配置
func LoadConfig() (*Config, error) {
	// Auto-generate config.yaml with a random API key if no config file exists
	ensureDefaultConfig()

	// 尝试加载.env文件
	if err := godotenv.Load(); err != nil {
		logrus.Debug("No .env file found, using environment variables")
	}

	config := &Config{
		// 设置默认值
		Port:               8002,
		Debug:              false,
		APIKey:             "0000",
		Models:             "claude-sonnet-4.6,claude-sonnet-4-5-20250929,claude-sonnet-4-20250514,claude-3-5-sonnet-20241022",
		SystemPromptInject: "",
		Timeout:            60,
		MaxInputLength:     200000,
		FP: FP{
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		},
		Vision: Vision{
			Enabled:   false,
			Mode:      "api",
			BaseURL:   "https://api.openai.com/v1/chat/completions",
			APIKey:    "",
			Model:     "gpt-4o-mini",
			Languages: "eng,chi_sim",
		},
	}

	applyYAMLConfig(config)
	applyEnvOverrides(config)

	// 验证必要的配置
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// validate 验证配置
func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	if c.APIKey == "" {
		return fmt.Errorf("API_KEY is required")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if c.MaxInputLength <= 0 {
		return fmt.Errorf("max input length must be positive")
	}

	if c.Vision.Enabled {
		if c.Vision.Mode == "" {
			c.Vision.Mode = "api"
		}
		if c.Vision.Mode != "api" {
			return fmt.Errorf("vision mode must be api (local OCR has been removed)")
		}
		if strings.TrimSpace(c.Vision.APIKey) == "" {
			return fmt.Errorf("VISION_API_KEY is required when vision is enabled")
		}
	}

	return nil
}

func applyYAMLConfig(config *Config) {
	for _, path := range []string{"config.yaml", "config.yml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var yc yamlConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			logrus.Warnf("Failed to parse %s: %v", path, err)
			return
		}
		if yc.Port > 0 {
			config.Port = yc.Port
		}
		config.Debug = yc.Debug
		if strings.TrimSpace(yc.APIKey) != "" {
			config.APIKey = yc.APIKey
		}
		if strings.TrimSpace(yc.Models) != "" {
			config.Models = yc.Models
		} else if strings.TrimSpace(yc.CursorModel) != "" {
			config.Models = yc.CursorModel
		}
		if strings.TrimSpace(yc.SystemPromptInject) != "" {
			config.SystemPromptInject = yc.SystemPromptInject
		}
		if yc.Timeout > 0 {
			config.Timeout = yc.Timeout
		}
		if yc.MaxInputLength > 0 {
			config.MaxInputLength = yc.MaxInputLength
		}
		if strings.TrimSpace(yc.Proxy) != "" {
			config.Proxy = yc.Proxy
		}
		if strings.TrimSpace(yc.Fingerprint.UserAgent) != "" {
			config.FP.UserAgent = yc.Fingerprint.UserAgent
		}
		if yc.Vision.Mode != "" || yc.Vision.BaseURL != "" || yc.Vision.APIKey != "" || yc.Vision.Model != "" || yc.Vision.Enabled || visionKeyExists(data) {
			config.Vision = mergeVision(config.Vision, yc.Vision)
			if visionKeyExists(data) {
				config.Vision.Enabled = true
			}
			if strings.Contains(string(data), "enabled: false") {
				config.Vision.Enabled = false
			}
		}
		return
	}
}

func applyEnvOverrides(config *Config) {
	config.Port = getEnvAsInt("PORT", config.Port)
	config.Debug = getEnvAsBool("DEBUG", config.Debug)
	config.APIKey = getEnv("API_KEY", config.APIKey)
	config.Models = getEnv("MODELS", config.Models)
	config.SystemPromptInject = getEnv("SYSTEM_PROMPT_INJECT", config.SystemPromptInject)
	config.Timeout = getEnvAsInt("TIMEOUT", config.Timeout)
	config.MaxInputLength = getEnvAsInt("MAX_INPUT_LENGTH", config.MaxInputLength)
	config.FP.UserAgent = getEnv("USER_AGENT", config.FP.UserAgent)

	config.Vision.Enabled = getEnvAsBool("VISION_ENABLED", config.Vision.Enabled)
	config.Vision.Mode = getEnv("VISION_MODE", config.Vision.Mode)
	config.Vision.BaseURL = getEnv("VISION_BASE_URL", config.Vision.BaseURL)
	config.Vision.APIKey = getEnv("VISION_API_KEY", config.Vision.APIKey)
	config.Vision.Model = getEnv("VISION_MODEL", config.Vision.Model)
	config.Vision.Languages = getEnv("VISION_LANGUAGES", config.Vision.Languages)

	config.Proxy = getEnv("PROXY", config.Proxy)
}

func mergeVision(base Vision, overlay Vision) Vision {
	merged := base
	if overlay.Mode != "" {
		merged.Mode = overlay.Mode
	}
	if overlay.BaseURL != "" {
		merged.BaseURL = overlay.BaseURL
	}
	if overlay.APIKey != "" {
		merged.APIKey = overlay.APIKey
	}
	if overlay.Model != "" {
		merged.Model = overlay.Model
	}
	if overlay.Languages != "" {
		merged.Languages = overlay.Languages
	}
	if overlay.Enabled {
		merged.Enabled = true
	}
	return merged
}

func visionKeyExists(data []byte) bool {
	return strings.Contains(string(data), "vision:")
}

// GetModels 获取模型列表
func (c *Config) GetModels() []string {
	models := strings.Split(c.Models, ",")
	result := make([]string, 0, len(models))
	for _, model := range models {
		if trimmed := strings.TrimSpace(model); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// IsValidModel 检查模型是否有效
func (c *Config) IsValidModel(model string) bool {
	validModels := c.GetModels()
	for _, validModel := range validModels {
		if validModel == model {
			return true
		}
	}
	return false
}

// ToJSON 将配置序列化为JSON（用于调试）
func (c *Config) ToJSON() string {
	// 创建一个副本，隐藏敏感信息
	safeCfg := *c
	safeCfg.APIKey = "***"

	data, err := json.MarshalIndent(safeCfg, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling config: %v", err)
	}
	return string(data)
}

// 辅助函数

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt 获取环境变量并转换为int
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		logrus.Warnf("Invalid integer value for %s: %s, using default: %d", key, valueStr, defaultValue)
		return defaultValue
	}

	return value
}

// getEnvAsBool 获取环境变量并转换为bool
func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		logrus.Warnf("Invalid boolean value for %s: %s, using default: %t", key, valueStr, defaultValue)
		return defaultValue
	}

	return value
}
