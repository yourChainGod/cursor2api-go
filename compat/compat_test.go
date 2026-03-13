package compat

import (
	"strings"
	"testing"

	"cursor2api-go/config"
)

func TestParseToolCallsWithEmbeddedCodeFence(t *testing.T) {
	response := "Before call\n```json action\n{\n  \"tool\": \"Write\",\n  \"parameters\": {\n    \"file_path\": \"main.go\",\n    \"content\": \"package main\\n\\nfunc main() {\\n    println(\\\"hi\\\")\\n}\\n```not_a_fence\\n\"\n  }\n}\n```\nAfter call"

	toolCalls, cleanText := ParseToolCalls(response)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Write" {
		t.Fatalf("expected tool Write, got %s", toolCalls[0].Name)
	}
	if _, ok := toolCalls[0].Arguments["content"]; !ok {
		t.Fatalf("expected content argument to survive embedded code fence")
	}
	if cleanText != "Before call\n\nAfter call" && cleanText != "Before call\nAfter call" {
		t.Fatalf("unexpected clean text: %q", cleanText)
	}
}

func TestConvertOpenAIToAnthropicMergesToolCalls(t *testing.T) {
	request := &OpenAIChatRequest{
		Model: "claude-sonnet-4.6",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Read the file"},
			{Role: "assistant", ToolCalls: []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "Read", Arguments: `{"file_path":"main.go"}`}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "package main"},
		},
		Tools: []map[string]interface{}{{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "Read",
				"description": "Read a file",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file_path": map[string]interface{}{"type": "string"},
					},
				},
			},
		}},
	}

	anthropic := ConvertOpenAIToAnthropic(request)
	if anthropic.System != "You are helpful" {
		t.Fatalf("expected system prompt to be preserved")
	}
	if len(anthropic.Messages) != 3 {
		t.Fatalf("expected 3 anthropic messages, got %d", len(anthropic.Messages))
	}
	if len(anthropic.Tools) != 1 || anthropic.Tools[0].Name != "Read" {
		t.Fatalf("expected tool definition to be preserved")
	}
}

func TestConvertAnthropicToCursorRequestInjectsToolInstructions(t *testing.T) {
	cfg := &config.Config{Models: "claude-sonnet-4.6", SystemPromptInject: "Injected system prompt"}
	request := &AnthropicRequest{
		Model:    "claude-sonnet-4.6",
		System:   "Original system prompt",
		Messages: []AnthropicMessage{{Role: "user", Content: "List files"}},
		Tools: []AnthropicTool{{
			Name:        "Bash",
			Description: "Run a shell command",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string"},
				},
			},
		}},
	}

	cursorReq := ConvertAnthropicToCursorRequest(request, cfg)
	if len(cursorReq.Messages) < 3 {
		t.Fatalf("expected tool instructions + few-shot + user message, got %d messages", len(cursorReq.Messages))
	}
	first := cursorReq.Messages[0].Parts[0].Text
	if first == "" || cursorReq.Messages[0].Role != "user" {
		t.Fatalf("expected first cursor message to contain injected tool instructions")
	}
	if !containsAll(first, []string{"Original system prompt", "Injected system prompt", "Bash"}) {
		t.Fatalf("expected system prompt and tool name in first message, got: %s", first)
	}
}

func TestConvertAnthropicToCursorRequestNoToolsReframesAndCleansHistory(t *testing.T) {
	cfg := &config.Config{Models: "claude-sonnet-4.6", SystemPromptInject: "Injected system prompt"}
	request := &AnthropicRequest{
		Model:  "claude-sonnet-4.6",
		System: "Original system prompt",
		Messages: []AnthropicMessage{
			{Role: "assistant", Content: "I am Cursor's support assistant and I only answer Cursor IDE questions."},
			{Role: "user", Content: "Explain TCP keepalive."},
		},
	}

	cursorReq := ConvertAnthropicToCursorRequest(request, cfg)
	if len(cursorReq.Messages) != 2 {
		t.Fatalf("expected 2 cursor messages, got %d", len(cursorReq.Messages))
	}
	assistantText := cursorReq.Messages[0].Parts[0].Text
	if assistantText != "I understand. Let me help you with that." {
		t.Fatalf("expected assistant refusal history to be cleaned, got %q", assistantText)
	}
	userText := cursorReq.Messages[1].Parts[0].Text
	if !containsAll(userText, []string{"versatile AI assistant", "Original system prompt", "Injected system prompt", "Explain TCP keepalive."}) {
		t.Fatalf("expected reframed user prompt with system text, got %q", userText)
	}
}

func TestParseResponseSegmentsThinkingAndTool(t *testing.T) {
	response := "Intro\n<thinking>Plan carefully</thinking>\n```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```\nDone"
	segments := ParseResponseSegments(response)
	if len(segments) < 4 {
		t.Fatalf("expected at least 4 segments, got %d: %#v", len(segments), segments)
	}
	if segments[0].Type != "text" || !strings.Contains(segments[0].Text, "Intro") {
		t.Fatalf("unexpected first segment: %#v", segments[0])
	}
	foundThinking := false
	foundTool := false
	foundDone := false
	for _, seg := range segments {
		if seg.Type == "thinking" && seg.Thinking == "Plan carefully" {
			foundThinking = true
		}
		if seg.Type == "tool_use" && seg.ToolCall != nil && seg.ToolCall.Name == "Read" {
			foundTool = true
		}
		if seg.Type == "text" && strings.Contains(seg.Text, "Done") {
			foundDone = true
		}
	}
	if !foundThinking || !foundTool || !foundDone {
		t.Fatalf("missing expected segments: thinking=%v tool=%v done=%v segments=%#v", foundThinking, foundTool, foundDone, segments)
	}
}

func TestConvertAnthropicToCursorRequestThinkingHint(t *testing.T) {
	cfg := &config.Config{Models: "claude-sonnet-4.6"}
	request := &AnthropicRequest{
		Model:    "claude-sonnet-4.6",
		Messages: []AnthropicMessage{{Role: "user", Content: "Solve this"}},
		Thinking: &ThinkingConfig{Type: "enabled", BudgetTokens: 2048},
	}
	cursorReq := ConvertAnthropicToCursorRequest(request, cfg)
	if len(cursorReq.Messages) == 0 {
		t.Fatalf("expected cursor messages")
	}
	text := cursorReq.Messages[0].Parts[0].Text
	if !containsAll(text, []string{"<thinking>", "2048 tokens"}) {
		t.Fatalf("expected thinking hint in user message, got %q", text)
	}
}

func TestTruncateRunesPreservesUTF8Runes(t *testing.T) {
	input := "常量 | 值 | 用途"
	got := truncateRunes(input, 7)
	if strings.ContainsRune(got, '\ufffd') {
		t.Fatalf("unexpected replacement rune in %q", got)
	}
}

func TestStreamResponseParserStreamsTextIncrementally(t *testing.T) {
	p := NewStreamResponseParser(false, false)
	p.Feed("Hello")
	if got := p.ConsumeEvents(); len(got) != 0 {
		t.Fatalf("expected no immediate flush for tiny buffer, got %#v", got)
	}
	p.Feed(" world, this should flush incrementally")
	events := p.ConsumeEvents()
	if len(events) == 0 || events[0].Type != "text" {
		t.Fatalf("expected text event, got %#v", events)
	}
	p.Finish()
	events = append(events, p.ConsumeEvents()...)
	joined := ""
	for _, ev := range events {
		if ev.Type == "text" {
			joined += ev.Text
		}
	}
	if joined != "Hello world, this should flush incrementally" {
		t.Fatalf("unexpected joined text %q", joined)
	}
}

func TestStreamResponseParserParsesThinkingAndToolAcrossFeeds(t *testing.T) {
	p := NewStreamResponseParser(true, true)
	p.Feed("Before <thinking>Plan")
	p.Feed(" carefully</thinking>```json action\n{\n  \"tool\": \"Read\",\n  \"parameters\": {\n    \"file_path\": \"main.go\"\n  }\n}\n```After")
	p.Finish()
	events := p.ConsumeEvents()
	foundThinking := false
	foundTool := false
	foundAfter := false
	for _, ev := range events {
		if ev.Type == "thinking" && ev.Thinking == "Plan carefully" {
			foundThinking = true
		}
		if ev.Type == "tool_use" && ev.ToolCall != nil && ev.ToolCall.Name == "Read" {
			foundTool = true
		}
		if ev.Type == "text" && strings.Contains(ev.Text, "After") {
			foundAfter = true
		}
	}
	if !foundThinking || !foundTool || !foundAfter {
		t.Fatalf("unexpected parser events: %#v", events)
	}
}

func containsAll(text string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}
