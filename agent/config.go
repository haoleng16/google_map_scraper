package agent

import "fmt"

// AgentConfig holds all configuration for the WhatsApp AI Agent.
type AgentConfig struct {
	// DeepSeek API settings.
	DeepSeekAPIKey  string  `json:"deepseek_api_key"`
	DeepSeekBaseURL string  `json:"deepseek_base_url"` // Default: https://api.deepseek.com
	Model           string  `json:"model"`              // Default: deepseek-chat
	EmbeddingModel  string  `json:"embedding_model"`    // Default: deepseek-chat

	// Safety settings for auto-reply.
	MaxRepliesPerHour    int `json:"max_replies_per_hour"`     // Default: 10
	ReplyDelayMinSeconds int `json:"reply_delay_min_seconds"`  // Default: 3
	ReplyDelayMaxSeconds int `json:"reply_delay_max_seconds"`  // Default: 10
	ContextWindowSize    int `json:"context_window_size"`      // Default: 20

	// RAG settings.
	ChunkSize    int `json:"chunk_size"`     // Default: 500
	ChunkOverlap int `json:"chunk_overlap"`  // Default: 50
	TopK         int `json:"top_k"`          // Default: 5

	// Rate limiting for LLM API calls.
	RateLimitPerSecond float64 `json:"rate_limit_per_second"` // Default: 2.0

	// Listener polling interval in seconds.
	PollIntervalSeconds int `json:"poll_interval_seconds"` // Default: 5
}

// DefaultConfig returns sensible defaults for personal/small team use.
func DefaultConfig() AgentConfig {
	return AgentConfig{
		DeepSeekBaseURL:     "https://api.deepseek.com",
		Model:               "deepseek-chat",
		EmbeddingModel:      "deepseek-chat",
		MaxRepliesPerHour:   10,
		ReplyDelayMinSeconds: 3,
		ReplyDelayMaxSeconds: 10,
		ContextWindowSize:   20,
		ChunkSize:           500,
		ChunkOverlap:        50,
		TopK:                5,
		RateLimitPerSecond:  2.0,
		PollIntervalSeconds: 5,
	}
}

// Normalize fills zero values with defaults.
func (c AgentConfig) Normalize() AgentConfig {
	d := DefaultConfig()
	if c.DeepSeekBaseURL == "" {
		c.DeepSeekBaseURL = d.DeepSeekBaseURL
	}
	if c.Model == "" {
		c.Model = d.Model
	}
	if c.EmbeddingModel == "" {
		c.EmbeddingModel = d.EmbeddingModel
	}
	if c.MaxRepliesPerHour <= 0 {
		c.MaxRepliesPerHour = d.MaxRepliesPerHour
	}
	if c.ReplyDelayMinSeconds <= 0 {
		c.ReplyDelayMinSeconds = d.ReplyDelayMinSeconds
	}
	if c.ReplyDelayMaxSeconds <= 0 {
		c.ReplyDelayMaxSeconds = d.ReplyDelayMaxSeconds
	}
	if c.ReplyDelayMinSeconds > c.ReplyDelayMaxSeconds {
		c.ReplyDelayMinSeconds, c.ReplyDelayMaxSeconds = c.ReplyDelayMaxSeconds, c.ReplyDelayMinSeconds
	}
	if c.ContextWindowSize <= 0 {
		c.ContextWindowSize = d.ContextWindowSize
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = d.ChunkSize
	}
	if c.ChunkOverlap < 0 {
		c.ChunkOverlap = 0
	}
	if c.TopK <= 0 {
		c.TopK = d.TopK
	}
	if c.RateLimitPerSecond <= 0 {
		c.RateLimitPerSecond = d.RateLimitPerSecond
	}
	if c.PollIntervalSeconds <= 0 {
		c.PollIntervalSeconds = d.PollIntervalSeconds
	}
	return c
}

// Validate checks that required fields are set.
func (c AgentConfig) Validate() error {
	if c.DeepSeekAPIKey == "" {
		return fmt.Errorf("deepseek_api_key is required")
	}
	return nil
}
