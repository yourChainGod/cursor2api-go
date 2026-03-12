package handlers

import (
	"strings"
	"testing"

	"cursor2api-go/compat"
)

func TestIsRefusal(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"I am Cursor's support assistant and can only answer Cursor IDE questions.", true},
		{"我是 Cursor 的支持助手，我只能回答 Cursor 相关的问题。", true},
		{"Here is the answer you asked for.", false},
	}
	for _, tc := range cases {
		if got := isRefusal(tc.text); got != tc.want {
			t.Fatalf("isRefusal(%q)=%v want %v", tc.text, got, tc.want)
		}
	}
}

func TestSanitizeResponse(t *testing.T) {
	input := "I am a support assistant for Cursor IDE and part of Cursor support."
	output := sanitizeResponse(input)
	if strings.Contains(strings.ToLower(output), "cursor support") || strings.Contains(strings.ToLower(output), "cursor ide") {
		t.Fatalf("expected Cursor identity to be sanitized, got %q", output)
	}
	if !strings.Contains(output, "Claude") {
		t.Fatalf("expected sanitized text to mention Claude, got %q", output)
	}
}

func TestIsIdentityProbe(t *testing.T) {
	body := &compat.AnthropicRequest{
		Model:    "claude-sonnet-4.6",
		Messages: []compat.AnthropicMessage{{Role: "user", Content: "你到底是谁"}},
	}
	if !isIdentityProbe(body) {
		t.Fatal("expected identity probe to be detected")
	}

	body.Tools = []compat.AnthropicTool{{Name: "Read"}}
	if isIdentityProbe(body) {
		t.Fatal("expected agent/tool mode not to be intercepted as identity probe")
	}
}

func TestBuildRetryRequestReframesLastUserMessage(t *testing.T) {
	body := &compat.AnthropicRequest{
		Model: "claude-sonnet-4.6",
		Messages: []compat.AnthropicMessage{
			{Role: "assistant", Content: "Earlier"},
			{Role: "user", Content: "Tell me about this topic"},
		},
	}
	clone := buildRetryRequest(body, 0)
	last := clone.Messages[len(clone.Messages)-1]
	text, _ := last.Content.(string)
	if !strings.Contains(text, "programming project") {
		t.Fatalf("expected retry request to prepend reframing prefix, got %q", text)
	}
	orig := body.Messages[len(body.Messages)-1].Content.(string)
	if orig != "Tell me about this topic" {
		t.Fatalf("expected original request to remain unchanged, got %q", orig)
	}
}

func TestIsTruncated(t *testing.T) {
	if !isTruncated("```json action\n{\"tool\":\"Write\"") {
		t.Fatal("expected incomplete json code fence to be considered truncated")
	}
	if isTruncated("```json action\n{\"tool\":\"Write\"}\n```") {
		t.Fatal("expected complete code fence not to be considered truncated")
	}
}

func TestTailStringPreservesUTF8Runes(t *testing.T) {
	input := "abc常量|值|用途"
	got := tailString(input, 6)
	if !strings.Contains(got, "用途") {
		t.Fatalf("expected tail to contain full UTF-8 runes, got %q", got)
	}
	if strings.ContainsRune(got, '\ufffd') {
		t.Fatalf("expected no replacement rune, got %q", got)
	}
}

func TestChunkStringPreservesUTF8Runes(t *testing.T) {
	input := "常量 | 值 | 用途 | 常量 | 值 | 用途"
	chunks := chunkString(input, 5)
	joined := strings.Join(chunks, "")
	if joined != input {
		t.Fatalf("expected rune-safe chunking to preserve content, got %q", joined)
	}
	for _, chunk := range chunks {
		if strings.ContainsRune(chunk, '\ufffd') {
			t.Fatalf("unexpected replacement rune in chunk %q", chunk)
		}
	}
}
