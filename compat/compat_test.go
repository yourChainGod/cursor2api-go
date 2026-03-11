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
	if !containsAll(userText, []string{"software development workflow", "Original system prompt", "Injected system prompt", "Explain TCP keepalive."}) {
		t.Fatalf("expected reframed user prompt with system text, got %q", userText)
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
