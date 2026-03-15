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

package middleware

import (
	"cursor2api-go/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthRequired 认证中间件。接收 Config 中已解析的 API Key，
// 确保 YAML / .env / 环境变量三种来源的 key 都能正确校验。
func AuthRequired(configuredAPIKey ...string) gin.HandlerFunc {
	// 从可变参数取出已解析的 key；若未传入则回退到 "0000"
	expectedToken := "0000"
	if len(configuredAPIKey) > 0 && strings.TrimSpace(configuredAPIKey[0]) != "" {
		expectedToken = configuredAPIKey[0]
	}

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := ""

		switch {
		case strings.HasPrefix(authHeader, "Bearer "):
			token = strings.TrimPrefix(authHeader, "Bearer ")
		case c.GetHeader("x-api-key") != "":
			token = c.GetHeader("x-api-key")
		case c.GetHeader("anthropic-api-key") != "":
			token = c.GetHeader("anthropic-api-key")
		case authHeader == "":
			errorResponse := models.NewErrorResponse(
				"Missing authorization header or API key",
				"authentication_error",
				"missing_auth",
			)
			c.JSON(http.StatusUnauthorized, errorResponse)
			c.Abort()
			return
		default:
			errorResponse := models.NewErrorResponse(
				"Invalid authorization format. Expected 'Bearer <token>' or x-api-key/anthropic-api-key",
				"authentication_error",
				"invalid_auth_format",
			)
			c.JSON(http.StatusUnauthorized, errorResponse)
			c.Abort()
			return
		}

		if token != expectedToken {
			errorResponse := models.NewErrorResponse(
				"Invalid API key",
				"authentication_error",
				"invalid_api_key",
			)
			c.JSON(http.StatusUnauthorized, errorResponse)
			c.Abort()
			return
		}

		// 认证通过，继续处理请求
		c.Next()
	}
}
