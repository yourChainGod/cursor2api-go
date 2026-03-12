package handlers

import (
	"bytes"
	"context"
	"cursor2api-go/compat"
	"cursor2api-go/config"
	"cursor2api-go/middleware"
	"cursor2api-go/models"
	"cursor2api-go/services"
	"cursor2api-go/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type cursorExecutor interface {
	ChatCompletionWithCursorRequest(ctx context.Context, payload *models.CursorRequest) (<-chan interface{}, error)
}

// Handler 处理器结构
type Handler struct {
	config        *config.Config
	cursorService cursorExecutor
	docsContent   []byte
}

// NewHandler 创建新的处理器
func NewHandler(cfg *config.Config) (*Handler, error) {
	cursorService, err := services.NewCursorService(cfg)
	if err != nil {
		return nil, err
	}

	docsPath := "static/docs.html"
	var docsContent []byte
	if data, err := os.ReadFile(docsPath); err == nil {
		docsContent = data
	} else {
		docsContent = []byte(defaultDocsHTML)
	}

	return &Handler{
		config:        cfg,
		cursorService: cursorService,
		docsContent:   docsContent,
	}, nil
}

// ListModels 列出可用模型
func (h *Handler) ListModels(c *gin.Context) {
	modelNames := h.config.GetModels()
	modelList := make([]models.Model, 0, len(modelNames))
	seen := map[string]bool{}

	for _, modelID := range modelNames {
		if seen[modelID] {
			continue
		}
		seen[modelID] = true

		modelConfig, exists := models.GetModelConfig(modelID)
		model := models.Model{
			ID:      modelID,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "cursor2api",
		}
		if exists {
			model.MaxTokens = modelConfig.MaxTokens
			model.ContextWindow = modelConfig.ContextWindow
		}
		modelList = append(modelList, model)
	}

	c.JSON(http.StatusOK, models.ModelsResponse{Object: "list", Data: modelList})
}

// ChatCompletions handles OpenAI chat completions with tool/responses compatibility.
func (h *Handler) ChatCompletions(c *gin.Context) {
	var body compat.OpenAIChatRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Invalid request format", "invalid_request_error", "invalid_json"))
		return
	}
	if len(body.Messages) == 0 {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Messages cannot be empty", "invalid_request_error", "missing_messages"))
		return
	}

	anthropicReq := compat.ConvertOpenAIToAnthropic(&body)
	if isIdentityProbe(anthropicReq) {
		if body.Stream {
			h.handleOpenAIMockStream(c, &body, claudeMockIdentityResponse)
		} else {
			h.handleOpenAIMockNonStream(c, &body, claudeMockIdentityResponse)
		}
		return
	}
	if body.Stream {
		h.streamOpenAI(c, &body, anthropicReq)
		return
	}
	h.nonStreamOpenAI(c, &body, anthropicReq)
}

// Messages handles Anthropic Messages API compatibility.
func (h *Handler) Messages(c *gin.Context) {
	var body compat.AnthropicRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Invalid request format", "invalid_request_error", "invalid_json"))
		return
	}
	if len(body.Messages) == 0 {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Messages cannot be empty", "invalid_request_error", "missing_messages"))
		return
	}
	if body.MaxTokens <= 0 {
		body.MaxTokens = 8192
	}

	if isIdentityProbe(&body) {
		if body.Stream {
			h.handleMockIdentityStream(c, &body, claudeMockIdentityResponse)
		} else {
			h.handleMockIdentityNonStream(c, &body, claudeMockIdentityResponse)
		}
		return
	}

	if body.Stream {
		h.streamAnthropic(c, &body)
		return
	}
	h.nonStreamAnthropic(c, &body)
}

// CountTokens provides a lightweight Anthropic-compatible token counter.
func (h *Handler) CountTokens(c *gin.Context) {
	var body compat.AnthropicRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Invalid request format", "invalid_request_error", "invalid_json"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": compat.EstimateAnthropicInputTokens(&body)})
}

// Responses handles OpenAI Responses API requests by converting them into Chat Completions.
func (h *Handler) Responses(c *gin.Context) {
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, models.NewErrorResponse("Invalid request format", "invalid_request_error", "invalid_json"))
		return
	}
	body := compat.ResponsesToChatCompletions(raw)
	anthropicReq := compat.ConvertOpenAIToAnthropic(body)
	if isIdentityProbe(anthropicReq) {
		if body.Stream {
			h.handleOpenAIMockStream(c, body, claudeMockIdentityResponse)
		} else {
			h.handleOpenAIMockNonStream(c, body, claudeMockIdentityResponse)
		}
		return
	}
	if body.Stream {
		h.streamOpenAI(c, body, anthropicReq)
		return
	}
	h.nonStreamOpenAI(c, body, anthropicReq)
}

func (h *Handler) nonStreamAnthropic(c *gin.Context, body *compat.AnthropicRequest) {
	result, err := h.executeAnthropicRequest(c.Request.Context(), body)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	content, stopReason := buildAnthropicContentBlocks(body, result.Text)
	if len(content) == 0 {
		content = append(content, compat.AnthropicContentBlock{Type: "text", Text: ""})
	}

	usage := compat.AnthropicUsage{InputTokens: compat.EstimateAnthropicInputTokens(body), OutputTokens: estimateOutputTokens(result)}
	if result.Usage.PromptTokens > 0 {
		usage.InputTokens = result.Usage.PromptTokens
	}
	if result.Usage.CompletionTokens > 0 {
		usage.OutputTokens = result.Usage.CompletionTokens
	}

	response := compat.AnthropicResponse{
		ID:           "msg_" + randomID(24),
		Type:         "message",
		Role:         "assistant",
		Content:      content,
		Model:        body.Model,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage:        usage,
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) streamAnthropic(c *gin.Context, body *compat.AnthropicRequest) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	messageID := "msg_" + randomID(24)
	writeAnthropicSSE(c, "message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         body.Model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         gin.H{"input_tokens": compat.EstimateAnthropicInputTokens(body), "output_tokens": 0},
		},
	})

	cursorReq := compat.ConvertAnthropicToCursorRequest(body, h.config)
	chatGenerator, err := h.cursorService.ChatCompletionWithCursorRequest(c.Request.Context(), &cursorReq)
	if err != nil {
		writeAnthropicSSE(c, "error", gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": err.Error()}})
		return
	}

	parser := compat.NewStreamResponseParser(len(body.Tools) > 0, body.Thinking != nil && strings.EqualFold(body.Thinking.Type, "enabled"))
	var fullText bytes.Buffer
	var sanitizeBuffer strings.Builder
	var usage models.Usage
	blockIndex := 0
	textBlockOpen := false
	textBlockIndex := -1
	toolSeen := false

	flushSanitized := func(force bool) {
		raw := sanitizeBuffer.String()
		if raw == "" {
			return
		}
		// Only flush when we have enough text for reliable pattern matching, or on force
		if !force && len(raw) < 200 {
			return
		}
		cleanText := sanitizeResponse(raw)
		if len(body.Tools) > 0 && isRefusal(raw) {
			cleanText = ""
		}
		sanitizeBuffer.Reset()
		if strings.TrimSpace(cleanText) == "" {
			return
		}
		if !textBlockOpen {
			textBlockIndex = blockIndex
			writeAnthropicSSE(c, "content_block_start", gin.H{"type": "content_block_start", "index": textBlockIndex, "content_block": gin.H{"type": "text", "text": ""}})
			textBlockOpen = true
			blockIndex++
		}
		writeAnthropicSSE(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": textBlockIndex, "delta": gin.H{"type": "text_delta", "text": cleanText}})
	}
	emitText := func(text string) {
		sanitizeBuffer.WriteString(text)
		flushSanitized(false)
	}
	closeText := func() {
		flushSanitized(true)
		if textBlockOpen {
			writeAnthropicSSE(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": textBlockIndex})
			textBlockOpen = false
			textBlockIndex = -1
		}
	}
	emitSegments := func(segments []compat.ResponseSegment) {
		for _, seg := range segments {
			switch seg.Type {
			case "text":
				emitText(seg.Text)
			case "thinking":
				closeText()
				if strings.TrimSpace(seg.Thinking) != "" {
					writeAnthropicThinkingBlock(c, blockIndex, seg.Thinking)
					blockIndex++
				}
			case "tool_use":
				closeText()
				if seg.ToolCall == nil {
					continue
				}
				toolSeen = true
				toolID := "toolu_" + randomID(24)
				writeAnthropicSSE(c, "content_block_start", gin.H{"type": "content_block_start", "index": blockIndex, "content_block": gin.H{"type": "tool_use", "id": toolID, "name": seg.ToolCall.Name, "input": gin.H{}}})
				argsJSON, _ := json.Marshal(seg.ToolCall.Arguments)
				for _, chunk := range chunkString(string(argsJSON), 128) {
					writeAnthropicSSE(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": blockIndex, "delta": gin.H{"type": "input_json_delta", "partial_json": chunk}})
				}
				writeAnthropicSSE(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": blockIndex})
				blockIndex++
			}
		}
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case item, ok := <-chatGenerator:
			if !ok {
				parser.Finish()
				emitSegments(parser.ConsumeEvents())
				closeText()
				result := collectedCursorOutput{Text: fullText.String(), Usage: usage}
				stopReason := "end_turn"
				if toolSeen {
					stopReason = "tool_use"
				} else if len(body.Tools) > 0 && isTruncated(result.Text) {
					stopReason = "max_tokens"
				}
				writeAnthropicSSE(c, "message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": stopReason, "stop_sequence": nil}, "usage": gin.H{"output_tokens": estimateOutputTokens(result)}})
				writeAnthropicSSE(c, "message_stop", gin.H{"type": "message_stop"})
				return
			}
			switch v := item.(type) {
			case string:
				fullText.WriteString(v)
				parser.Feed(v)
				emitSegments(parser.ConsumeEvents())
			case models.Usage:
				usage = v
			case error:
				closeText()
				writeAnthropicSSE(c, "error", gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": v.Error()}})
				return
			}
		}
	}
}

func (h *Handler) nonStreamOpenAI(c *gin.Context, body *compat.OpenAIChatRequest, anthropicReq *compat.AnthropicRequest) {
	result, err := h.executeAnthropicRequest(c.Request.Context(), anthropicReq)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	finishReason := "stop"
	message := compat.OpenAIAssistantText{Role: "assistant", Content: sanitizeResponse(result.Text)}
	if len(body.Tools) > 0 {
		toolCalls, cleanText := compat.ParseToolCalls(result.Text)
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
			if isRefusal(cleanText) {
				cleanText = ""
			}
			message.Content = nullableString(sanitizeResponse(cleanText))
			message.ToolCalls = make([]compat.OpenAIToolCall, 0, len(toolCalls))
			for _, tc := range toolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				message.ToolCalls = append(message.ToolCalls, compat.OpenAIToolCall{
					ID:   "call_" + randomID(24),
					Type: "function",
					Function: compat.OpenAIFunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
	}

	promptTokens := compat.EstimateAnthropicInputTokens(anthropicReq)
	if result.Usage.PromptTokens > 0 {
		promptTokens = result.Usage.PromptTokens
	}
	completionTokens := estimateOutputTokens(result)
	if result.Usage.CompletionTokens > 0 {
		completionTokens = result.Usage.CompletionTokens
	}

	response := compat.OpenAIChatCompletion{
		ID:      "chatcmpl-" + randomID(24),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   body.Model,
		Choices: []compat.OpenAIChatChoice{{Index: 0, Message: message, FinishReason: finishReason}},
		Usage: compat.OpenAIUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) streamOpenAI(c *gin.Context, body *compat.OpenAIChatRequest, anthropicReq *compat.AnthropicRequest) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	id := "chatcmpl-" + randomID(24)
	created := time.Now().Unix()
	writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   body.Model,
		Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{Role: "assistant", Content: ""}, FinishReason: nil}},
	})

	cursorReq := compat.ConvertAnthropicToCursorRequest(anthropicReq, h.config)
	chatGenerator, err := h.cursorService.ChatCompletionWithCursorRequest(c.Request.Context(), &cursorReq)
	if err != nil {
		writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{Content: "\n\n[Error: " + err.Error() + "]"}, FinishReason: "stop"}}})
		_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	parser := compat.NewStreamResponseParser(len(body.Tools) > 0, false)
	var fullText bytes.Buffer
	var openaiSanitizeBuf strings.Builder
	var toolSeen bool
	toolCallIndex := 0

	flushOpenAISanitized := func(force bool) {
		raw := openaiSanitizeBuf.String()
		if raw == "" {
			return
		}
		if !force && len(raw) < 200 {
			return
		}
		cleanText := sanitizeResponse(raw)
		if len(body.Tools) > 0 && isRefusal(raw) {
			cleanText = ""
		}
		openaiSanitizeBuf.Reset()
		if strings.TrimSpace(cleanText) == "" {
			return
		}
		writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{Content: cleanText}, FinishReason: nil}}})
	}

	emitSegments := func(segments []compat.ResponseSegment) {
		for _, seg := range segments {
			switch seg.Type {
			case "text":
				openaiSanitizeBuf.WriteString(seg.Text)
				flushOpenAISanitized(false)
			case "tool_use":
				if seg.ToolCall == nil {
					continue
				}
				toolSeen = true
				callID := "call_" + randomID(24)
				idx := toolCallIndex
				toolCallIndex++
				writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{ToolCalls: []compat.OpenAIStreamToolCall{{Index: idx, ID: callID, Type: "function", Function: compat.OpenAIStreamFunctionPayload{Name: seg.ToolCall.Name, Arguments: ""}}}}, FinishReason: nil}}})
				argsJSON, _ := json.Marshal(seg.ToolCall.Arguments)
				for _, chunk := range chunkString(string(argsJSON), 128) {
					writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{ToolCalls: []compat.OpenAIStreamToolCall{{Index: idx, Function: compat.OpenAIStreamFunctionPayload{Arguments: chunk}}}}, FinishReason: nil}}})
				}
			}
		}
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case item, ok := <-chatGenerator:
			if !ok {
				parser.Finish()
				emitSegments(parser.ConsumeEvents())
				flushOpenAISanitized(true)
				finishReason := "stop"
				if toolSeen {
					finishReason = "tool_calls"
				}
				writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{}, FinishReason: finishReason}}})
				_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
				return
			}
			switch v := item.(type) {
			case string:
				fullText.WriteString(v)
				parser.Feed(v)
				emitSegments(parser.ConsumeEvents())
			case models.Usage:
				// OpenAI stream finish usage is not emitted progressively here.
				_ = v
			case error:
				writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: body.Model, Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{Content: "\n\n[Error: " + v.Error() + "]"}, FinishReason: "stop"}}})
				_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
				return
			}
		}
	}
}

func (h *Handler) handleMockIdentityStream(c *gin.Context, body *compat.AnthropicRequest, mockText string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	messageID := "msg_" + randomID(24)
	model := body.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	writeAnthropicSSE(c, "message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         gin.H{"input_tokens": 15, "output_tokens": 0},
		},
	})
	writeAnthropicTextBlock(c, 0, mockText)
	writeAnthropicSSE(c, "message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": gin.H{"output_tokens": 35}})
	writeAnthropicSSE(c, "message_stop", gin.H{"type": "message_stop"})
}

func (h *Handler) handleMockIdentityNonStream(c *gin.Context, body *compat.AnthropicRequest, mockText string) {
	model := body.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	c.JSON(http.StatusOK, compat.AnthropicResponse{
		ID:           "msg_" + randomID(24),
		Type:         "message",
		Role:         "assistant",
		Content:      []compat.AnthropicContentBlock{{Type: "text", Text: mockText}},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: nil,
		Usage:        compat.AnthropicUsage{InputTokens: 15, OutputTokens: 35},
	})
}

func (h *Handler) handleOpenAIMockStream(c *gin.Context, body *compat.OpenAIChatRequest, mockText string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	id := "chatcmpl-" + randomID(24)
	created := time.Now().Unix()
	writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   body.Model,
		Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{Role: "assistant", Content: mockText}, FinishReason: nil}},
	})
	writeOpenAISSE(c, compat.OpenAIChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   body.Model,
		Choices: []compat.OpenAIStreamChoice{{Index: 0, Delta: compat.OpenAIStreamDelta{}, FinishReason: "stop"}},
	})
	_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (h *Handler) handleOpenAIMockNonStream(c *gin.Context, body *compat.OpenAIChatRequest, mockText string) {
	c.JSON(http.StatusOK, compat.OpenAIChatCompletion{
		ID:      "chatcmpl-" + randomID(24),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   body.Model,
		Choices: []compat.OpenAIChatChoice{{Index: 0, Message: compat.OpenAIAssistantText{Role: "assistant", Content: mockText}, FinishReason: "stop"}},
		Usage:   compat.OpenAIUsage{PromptTokens: 15, CompletionTokens: 35, TotalTokens: 50},
	})
}

// ServeDocs 服务API文档页面
func (h *Handler) ServeDocs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", h.docsContent)
}

// Health 健康检查
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().Unix(), "version": "go-compat-2.0.0"})
}

type collectedCursorOutput struct {
	Text  string
	Usage models.Usage
}

func collectCursorOutput(ctx context.Context, chatGenerator <-chan interface{}) (collectedCursorOutput, error) {
	var buffer bytes.Buffer
	var usage models.Usage
	for {
		select {
		case <-ctx.Done():
			return collectedCursorOutput{}, fmt.Errorf("request cancelled")
		case item, ok := <-chatGenerator:
			if !ok {
				return collectedCursorOutput{Text: buffer.String(), Usage: usage}, nil
			}
			switch v := item.(type) {
			case string:
				buffer.WriteString(v)
			case models.Usage:
				usage = v
			case error:
				return collectedCursorOutput{}, v
			}
		}
	}
}

func estimateOutputTokens(result collectedCursorOutput) int {
	if result.Usage.CompletionTokens > 0 {
		return result.Usage.CompletionTokens
	}
	if result.Text == "" {
		return 0
	}
	return (len(result.Text) + 3) / 4
}

func randomID(length int) string {
	return utils.GenerateRandomString(length)
}

func buildAnthropicContentBlocks(body *compat.AnthropicRequest, text string) ([]compat.AnthropicContentBlock, string) {
	stopReason := "end_turn"
	if len(body.Tools) > 0 && isTruncated(text) {
		stopReason = "max_tokens"
	}

	thinkingEnabled := body.Thinking != nil && strings.EqualFold(body.Thinking.Type, "enabled")
	useSegments := thinkingEnabled || len(body.Tools) > 0
	segments := []compat.ResponseSegment{{Type: "text", Text: text}}
	if useSegments {
		segments = compat.ParseResponseSegments(text)
	}

	content := make([]compat.AnthropicContentBlock, 0, len(segments))
	toolSeen := false
	for _, seg := range segments {
		switch seg.Type {
		case "thinking":
			if thinkingEnabled && strings.TrimSpace(seg.Thinking) != "" {
				content = append(content, compat.AnthropicContentBlock{Type: "thinking", Thinking: seg.Thinking})
			}
		case "tool_use":
			if seg.ToolCall != nil {
				toolSeen = true
				content = append(content, compat.AnthropicContentBlock{Type: "tool_use", ID: "toolu_" + randomID(24), Name: seg.ToolCall.Name, Input: seg.ToolCall.Arguments})
			}
		case "text":
			cleanText := sanitizeResponse(seg.Text)
			if len(body.Tools) > 0 && isRefusal(seg.Text) {
				cleanText = ""
			}
			if strings.TrimSpace(cleanText) != "" {
				content = append(content, compat.AnthropicContentBlock{Type: "text", Text: cleanText})
			}
		}
	}

	if toolSeen {
		stopReason = "tool_use"
	}
	if len(content) == 0 {
		content = append(content, compat.AnthropicContentBlock{Type: "text", Text: sanitizeResponse(text)})
	}
	return content, stopReason
}

func chunkString(value string, size int) []string {
	if value == "" || size <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func writeAnthropicTextBlock(c *gin.Context, index int, text string) {
	writeAnthropicSSE(c, "content_block_start", gin.H{"type": "content_block_start", "index": index, "content_block": gin.H{"type": "text", "text": ""}})
	writeAnthropicSSE(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": index, "delta": gin.H{"type": "text_delta", "text": text}})
	writeAnthropicSSE(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": index})
}

func writeAnthropicThinkingBlock(c *gin.Context, index int, thinking string) {
	writeAnthropicSSE(c, "content_block_start", gin.H{"type": "content_block_start", "index": index, "content_block": gin.H{"type": "thinking", "thinking": ""}})
	writeAnthropicSSE(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": index, "delta": gin.H{"type": "thinking_delta", "thinking": thinking}})
	writeAnthropicSSE(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": index})
}

func writeAnthropicSSE(c *gin.Context, event string, payload interface{}) {
	blob, _ := json.Marshal(payload)
	_, _ = c.Writer.Write([]byte("event: " + event + "\n"))
	_, _ = c.Writer.Write([]byte("data: " + string(blob) + "\n\n"))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeOpenAISSE(c *gin.Context, payload compat.OpenAIChatCompletionChunk) {
	blob, _ := json.Marshal(payload)
	_, _ = c.Writer.Write([]byte("data: " + string(blob) + "\n\n"))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

const defaultDocsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cursor2API - Go Version</title>
    <style>
        body { font-family: sans-serif; max-width: 900px; margin: 40px auto; padding: 0 16px; }
        code, pre { background: #f6f8fa; padding: 2px 6px; border-radius: 6px; }
        pre { padding: 12px; overflow: auto; }
    </style>
</head>
<body>
    <h1>Cursor2API · Go compatibility build</h1>
    <p>Available endpoints:</p>
    <ul>
        <li><code>GET /v1/models</code></li>
        <li><code>POST /v1/chat/completions</code></li>
        <li><code>POST /v1/messages</code></li>
        <li><code>POST /v1/messages/count_tokens</code></li>
        <li><code>POST /v1/responses</code></li>
        <li><code>GET /health</code></li>
    </ul>
</body>
</html>`
