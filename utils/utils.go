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

package utils

import (
	"bufio"
	"context"
	"crypto/rand"
	"cursor2api-go/models"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// GenerateRandomString 生成指定长度的随机字符串
func GenerateRandomString(length int) string {
	if length <= 0 {
		return ""
	}

	byteLen := (length + 1) / 2
	bytes := make([]byte, byteLen)
	if _, err := rand.Read(bytes); err != nil {
		fallback := fmt.Sprintf("%d", time.Now().UnixNano())
		if len(fallback) >= length {
			return fallback[:length]
		}
		return fallback
	}

	encoded := hex.EncodeToString(bytes)
	if len(encoded) < length {
		encoded += GenerateRandomString(length - len(encoded))
	}

	return encoded[:length]
}

// ParseSSELine 解析SSE数据行
func ParseSSELine(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimSpace(line[6:]) // 去掉 'data: ' 前缀并去除前导空格
	}
	return ""
}

// ReadSSEStream 读取SSE流
func ReadSSEStream(ctx context.Context, resp *http.Response, output chan<- interface{}) error {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	defer resp.Body.Close()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		data := ParseSSELine(line)
		if data == "" {
			continue
		}

		if data == "[DONE]" {
			return nil
		}

		// 尝试解析JSON数据
		var eventData models.CursorEventData
		if err := json.Unmarshal([]byte(data), &eventData); err != nil {
			logrus.WithError(err).Debugf("Failed to parse SSE data: %s", data)
			continue
		}

		// 处理不同类型的事件
		switch eventData.Type {
		case "error":
			if eventData.ErrorText != "" {
				return fmt.Errorf("cursor API error: %s", eventData.ErrorText)
			}

		case "finish":
			if eventData.MessageMetadata != nil && eventData.MessageMetadata.Usage != nil {
				usage := models.Usage{
					PromptTokens:     eventData.MessageMetadata.Usage.InputTokens,
					CompletionTokens: eventData.MessageMetadata.Usage.OutputTokens,
					TotalTokens:      eventData.MessageMetadata.Usage.TotalTokens,
				}
				output <- usage
			}
			return nil

		default:
			if eventData.Delta != "" {
				output <- eventData.Delta
			}
		}
	}

	return scanner.Err()
}
