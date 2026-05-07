package agent

import (
	"context"
	"fmt"
	"time"

	oai "github.com/sashabaranov/go-openai"
)

// LLMClient wraps the OpenAI-compatible API client for DeepSeek.
type LLMClient struct {
	client    *oai.Client
	model     string
	maxTokens int
}

// NewLLMClient creates a new LLM client with the given configuration.
func NewLLMClient(apiKey, baseURL, model string) *LLMClient {
	config := oai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &LLMClient{
		client:    oai.NewClientWithConfig(config),
		model:     model,
		maxTokens: 2048,
	}
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// ToolCall represents a function call requested by the LLM.
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// ChatResult contains the LLM response and optional tool calls.
type ChatResult struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletion sends messages to the LLM and returns the response.
func (c *LLMClient) ChatCompletion(ctx context.Context, messages []ChatMessage, tools []oai.Tool) (*ChatResult, error) {
	oaiMsgs := make([]oai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		oaiMsgs[i] = oai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		}
	}

	req := oai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    oaiMsgs,
		Temperature: 0.7,
		MaxTokens:   c.maxTokens,
	}

	if len(tools) > 0 {
		req.Tools = tools
	}

	start := time.Now()
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm chat completion: %w", err)
	}
	_ = start

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	choice := resp.Choices[0]
	result := &ChatResult{
		Content: choice.Message.Content,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: tc.Function.Arguments,
		})
	}

	return result, nil
}
