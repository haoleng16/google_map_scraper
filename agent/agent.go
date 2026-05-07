package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	oai "github.com/sashabaranov/go-openai"

	"github.com/gosom/google-maps-scraper/whatsapp"
)

// MessageStore abstracts message persistence (implemented by agentDB in desktop layer).
type MessageStore interface {
	SaveMessage(phone, direction, content, role, toolName string, tokensUsed, createdAt int64) error
	GetMessages(phone string, limit int) ([]map[string]any, error)
	UpsertConversation(phone, name, status string, createdAt, updatedAt int64) error
	RecordStat(eventType, phone, detail string, tokensUsed, latencyMs, createdAt int64) error
}

// MessageSender abstracts WhatsApp message sending.
type MessageSender interface {
	OpenChat(phone string) bool
	SendTextInChat(text string) bool
}

// EventHandler is called to emit real-time events to the frontend.
type EventHandler func(eventType string, data map[string]any)

// RAGSearcher abstracts RAG search (implemented by rag.RAG).
type RAGSearcher interface {
	Search(ctx context.Context, queryText string) ([]string, error)
}

// Agent is the core AI agent that processes incoming WhatsApp messages,
// generates LLM responses, optionally calls tools, and sends replies.
type Agent struct {
	config  AgentConfig
	llm     *LLMClient
	convs   *ConversationStore
	store   MessageStore
	sender  MessageSender
	handler EventHandler
	rag     RAGSearcher
	scraper ScraperService

	// replyTracker limits replies per phone per hour.
	replyTracker map[string][]time.Time
}

// NewAgent creates a new Agent with the given configuration and dependencies.
func NewAgent(config AgentConfig, store MessageStore, sender MessageSender, handler EventHandler) *Agent {
	config = config.Normalize()
	return &Agent{
		config:       config,
		llm:          NewLLMClient(config.DeepSeekAPIKey, config.DeepSeekBaseURL, config.Model),
		convs:        NewConversationStore(config.ContextWindowSize),
		store:        store,
		sender:       sender,
		handler:      handler,
		replyTracker: make(map[string][]time.Time),
	}
}

// SetRAGSearcher sets the RAG searcher for knowledge base queries.
func (a *Agent) SetRAGSearcher(rag RAGSearcher) {
	a.rag = rag
}

// SetScraperService sets the scraper service for Google Maps tool.
func (a *Agent) SetScraperService(scraper ScraperService) {
	a.scraper = scraper
}

// HandleIncomingMessage processes an incoming WhatsApp message through the LLM
// and sends an auto-reply. This is the main entry point called by the listener.
func (a *Agent) HandleIncomingMessage(ctx context.Context, msg whatsapp.IncomingMessage) {
	now := time.Now().Unix()

	// Persist inbound message.
	if a.store != nil {
		_ = a.store.SaveMessage(msg.Phone, "inbound", msg.Text, "user", "", 0, now)
		_ = a.store.UpsertConversation(msg.Phone, msg.Name, "active", now, now)
		_ = a.store.RecordStat("message_received", msg.Phone, "", 0, 0, now)
	}

	// Emit frontend event.
	if a.handler != nil {
		a.handler("agent:message", map[string]any{
			"phone":     msg.Phone,
			"direction": "inbound",
			"content":   msg.Text,
			"name":      msg.Name,
			"timestamp": now,
		})
	}

	// Rate limit check.
	if !a.canReply(msg.Phone) {
		log.Printf("[agent] rate limited for %s, skipping reply", msg.Phone)
		return
	}

	// Simulate human-like delay before replying.
	delay := a.config.ReplyDelayMinSeconds + rand.Intn(a.config.ReplyDelayMaxSeconds-a.config.ReplyDelayMinSeconds+1)
	select {
	case <-time.After(time.Duration(delay) * time.Second):
	case <-ctx.Done():
		return
	}

	// Process through LLM.
	a.processAndReply(ctx, msg)
}

// Chat handles a direct chat message from the Wails frontend (not through WhatsApp).
func (a *Agent) Chat(ctx context.Context, userMsg string) (string, error) {
	system := BuildSystemPrompt("AI助手", "")

	messages := []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: userMsg},
	}

	result, err := a.llm.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}

	now := time.Now().Unix()
	if a.store != nil {
		_ = a.store.RecordStat("direct_chat", "", userMsg, int64(result.Usage.TotalTokens), 0, now)
	}

	return result.Content, nil
}

func (a *Agent) processAndReply(ctx context.Context, msg whatsapp.IncomingMessage) {
	// Add user message to conversation window.
	a.convs.Add(msg.Phone, ChatMessage{
		Role:    "user",
		Content: msg.Text,
	})

	// Build RAG context from knowledge base.
	ragContext := ""
	if a.rag != nil {
		chunks, err := a.rag.Search(ctx, msg.Text)
		if err != nil {
			log.Printf("[agent] RAG search error for %s: %v", msg.Phone, err)
		} else if len(chunks) > 0 {
			ragContext = BuildRAGContext(chunks)
		}
	}

	// Build system prompt.
	system := BuildSystemPrompt("AI客服", ragContext)

	// Build message list for LLM.
	history := a.convs.Get(msg.Phone)
	messages := make([]ChatMessage, 0, len(history)+1)
	messages = append(messages, ChatMessage{Role: "system", Content: system})
	messages = append(messages, history...)

	// Define available tools.
	tools := a.buildTools()

	start := time.Now()
	result, err := a.llm.ChatCompletion(ctx, messages, tools)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		log.Printf("[agent] LLM error for %s: %v", msg.Phone, err)
		if a.store != nil {
			_ = a.store.RecordStat("error", msg.Phone, err.Error(), 0, latency, time.Now().Unix())
		}
		return
	}

	// Record usage stats.
	now := time.Now().Unix()
	if a.store != nil {
		_ = a.store.RecordStat("llm_call", msg.Phone, "", int64(result.Usage.TotalTokens), latency, now)
	}

	// Handle tool calls if present.
	replyContent := result.Content
	if len(result.ToolCalls) > 0 {
		replyContent = a.handleToolCalls(ctx, msg.Phone, result.ToolCalls, replyContent)
	}

	if replyContent == "" {
		return
	}

	// Sanitize reply (trim quotes, whitespace).
	replyContent = SanitizeReply(replyContent)

	// Safety check: block sensitive or overly long content.
	if block := ShouldBlock(replyContent); block.Blocked {
		log.Printf("[agent] blocked reply to %s: %s", msg.Phone, block.Reason)
		if a.store != nil {
			_ = a.store.RecordStat("reply_blocked", msg.Phone, block.Reason, 0, 0, now)
		}
		return
	}

	// Record assistant reply.
	a.convs.Add(msg.Phone, ChatMessage{
		Role:    "assistant",
		Content: replyContent,
	})

	// Persist outbound message.
	if a.store != nil {
		_ = a.store.SaveMessage(msg.Phone, "outbound", replyContent, "assistant", "", int64(result.Usage.TotalTokens), now)
		_ = a.store.RecordStat("reply_sent", msg.Phone, "", int64(result.Usage.TotalTokens), latency, now)
	}

	// Send via WhatsApp.
	if a.sender != nil {
		if ok := a.sender.OpenChat(msg.Phone); !ok {
			log.Printf("[agent] failed to open chat for %s", msg.Phone)
			return
		}
		if ok := a.sender.SendTextInChat(replyContent); !ok {
			log.Printf("[agent] failed to send reply to %s", msg.Phone)
			return
		}
	}

	// Emit frontend event.
	if a.handler != nil {
		a.handler("agent:message", map[string]any{
			"phone":     msg.Phone,
			"direction": "outbound",
			"content":   replyContent,
			"timestamp": now,
		})
	}

	// Track reply for rate limiting.
	a.recordReply(msg.Phone)
}

func (a *Agent) buildTools() []oai.Tool {
	return []oai.Tool{
		{
			Type: oai.ToolTypeFunction,
			Function: &oai.FunctionDefinition{
				Name:        "search_maps",
				Description: "搜索 Google Maps 上的商家信息。需要提供关键词和地点。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword": map[string]any{
							"type":        "string",
							"description": "搜索关键词，如'餐厅'、'酒店'",
						},
						"location": map[string]any{
							"type":        "string",
							"description": "地点，如'上海'、'北京'",
						},
					},
					"required": []string{"keyword", "location"},
				},
			},
		},
		{
			Type: oai.ToolTypeFunction,
			Function: &oai.FunctionDefinition{
				Name:        "send_message",
				Description: "向客户发送一条消息。仅在你需要主动通知客户时使用。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{
							"type":        "string",
							"description": "要发送的消息文本",
						},
					},
					"required": []string{"text"},
				},
			},
		},
	}
}

func (a *Agent) handleToolCalls(ctx context.Context, phone string, calls []ToolCall, currentContent string) string {
	for _, tc := range calls {
		log.Printf("[agent] tool call: %s(%s) for %s", tc.Name, tc.Args, phone)

		var result string
		switch tc.Name {
		case "search_maps":
			result = a.toolSearchMaps(ctx, tc.Args)
		case "send_message":
			result = a.toolSendMessage(phone, tc.Args)
		default:
			result = fmt.Sprintf("未知工具: %s", tc.Name)
		}

		if a.store != nil {
			_ = a.store.RecordStat("tool_call", phone, tc.Name, 0, 0, time.Now().Unix())
		}

		if currentContent == "" && result != "" {
			currentContent = result
		}
	}
	return currentContent
}

func (a *Agent) toolSendMessage(phone, argsJSON string) string {
	var args struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("解析参数失败: %s", err)
	}

	if a.sender != nil {
		if ok := a.sender.OpenChat(phone); ok {
			if ok := a.sender.SendTextInChat(args.Text); ok {
				return "消息已发送"
			}
		}
	}
	return "消息发送失败"
}

// canReply checks if we haven't exceeded the hourly reply limit for this phone.
func (a *Agent) canReply(phone string) bool {
	cutoff := time.Now().Add(-time.Hour)
	replies := a.replyTracker[phone]

	valid := make([]time.Time, 0, len(replies))
	for _, t := range replies {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	a.replyTracker[phone] = valid

	return len(valid) < a.config.MaxRepliesPerHour
}

// recordReply tracks a reply for rate limiting.
func (a *Agent) recordReply(phone string) {
	a.replyTracker[phone] = append(a.replyTracker[phone], time.Now())
}
