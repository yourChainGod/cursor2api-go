package compat

// Anthropic Messages API types.
type AnthropicRequest struct {
	Model         string               `json:"model" binding:"required"`
	Messages      []AnthropicMessage   `json:"messages" binding:"required"`
	MaxTokens     int                  `json:"max_tokens,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	System        interface{}          `json:"system,omitempty"`
	Tools         []AnthropicTool      `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice `json:"tool_choice,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Thinking      *ThinkingConfig      `json:"thinking,omitempty"`
}

// ThinkingConfig controls Claude's extended thinking mode.
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // max thinking tokens
}

type AnthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role" binding:"required"`
	Content interface{} `json:"content" binding:"required"`
}

type AnthropicContentBlock struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Thinking  string                 `json:"thinking,omitempty"`
	Source    *AnthropicImageSource  `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data"`
}

type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence interface{}             `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

// OpenAI Chat Completions API types.
type OpenAIChatRequest struct {
	Model               string                   `json:"model" binding:"required"`
	Messages            []OpenAIMessage          `json:"messages" binding:"required"`
	Stream              bool                     `json:"stream,omitempty"`
	Temperature         *float64                 `json:"temperature,omitempty"`
	TopP                *float64                 `json:"top_p,omitempty"`
	MaxTokens           *int                     `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                     `json:"max_completion_tokens,omitempty"`
	Tools               []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice          interface{}              `json:"tool_choice,omitempty"`
	Stop                interface{}              `json:"stop,omitempty"`
}

type OpenAIMessage struct {
	Role       string           `json:"role" binding:"required"`
	Content    interface{}      `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatCompletion struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []OpenAIChatChoice `json:"choices"`
	Usage   OpenAIUsage        `json:"usage"`
}

type OpenAIChatChoice struct {
	Index        int                 `json:"index"`
	Message      OpenAIAssistantText `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type OpenAIAssistantText struct {
	Role             string           `json:"role"`
	Content          interface{}      `json:"content"`
	ReasoningContent interface{}      `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIToolCall `json:"tool_calls,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
}

type OpenAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        OpenAIStreamDelta `json:"delta"`
	FinishReason interface{}       `json:"finish_reason"`
}

type OpenAIStreamDelta struct {
	Role             string                 `json:"role,omitempty"`
	Content          interface{}            `json:"content,omitempty"`
	ReasoningContent interface{}            `json:"reasoning_content,omitempty"`
	ToolCalls        []OpenAIStreamToolCall `json:"tool_calls,omitempty"`
}

type OpenAIStreamToolCall struct {
	Index    int                         `json:"index"`
	ID       string                      `json:"id,omitempty"`
	Type     string                      `json:"type,omitempty"`
	Function OpenAIStreamFunctionPayload `json:"function"`
}

type OpenAIStreamFunctionPayload struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type ParsedToolCall struct {
	Name      string
	Arguments map[string]interface{}
}

type ResponseSegment struct {
	Type     string
	Text     string
	Thinking string
	ToolCall *ParsedToolCall
}
