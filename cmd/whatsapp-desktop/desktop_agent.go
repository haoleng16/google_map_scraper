package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gosom/google-maps-scraper/agent"
	"github.com/gosom/google-maps-scraper/rag"
	"github.com/gosom/google-maps-scraper/whatsapp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AgentService manages the AI agent lifecycle and Wails bindings.
type AgentService struct {
	app     *App
	db      *agentDB
	agent   *agent.Agent
	rag     *rag.RAG
	config  agent.AgentConfig
	running bool
}

// newAgentService creates a new AgentService bound to the given App.
func newAgentService(app *App, db *agentDB) *AgentService {
	return &AgentService{app: app, db: db}
}

// === Agent Control ===

// AgentStart enables the auto-reply agent with the given configuration.
func (a *App) AgentStart(config agent.AgentConfig) error {
	if a.agentSvc == nil {
		return fmt.Errorf("agent service not initialized")
	}
	return a.agentSvc.start(config)
}

// AgentStop disables the auto-reply agent.
func (a *App) AgentStop() error {
	if a.agentSvc == nil {
		return fmt.Errorf("agent service not initialized")
	}
	return a.agentSvc.stop()
}

// AgentStatus returns the current agent state.
func (a *App) AgentStatus() map[string]any {
	if a.agentSvc == nil {
		return map[string]any{"running": false, "error": "not initialized"}
	}
	return a.agentSvc.status()
}

// === Conversations ===

// AgentConversations returns all active conversations.
func (a *App) AgentConversations() ([]map[string]any, error) {
	if a.agentDB == nil {
		return nil, fmt.Errorf("agent db not initialized")
	}
	return a.agentDB.GetConversations()
}

// AgentMessages returns message history for a phone number.
func (a *App) AgentMessages(phone string, limit int) ([]map[string]any, error) {
	if a.agentDB == nil {
		return nil, fmt.Errorf("agent db not initialized")
	}
	return a.agentDB.GetMessages(phone, limit)
}

// AgentSendManual sends a manual message to a contact (bypasses LLM).
func (a *App) AgentSendManual(phone, text string) error {
	if text == "" {
		return fmt.Errorf("message text is required")
	}
	if ok := a.service.Sender().OpenChat(phone); !ok {
		return fmt.Errorf("failed to open chat for %s", phone)
	}
	if ok := a.service.Sender().SendTextInChat(text); !ok {
		return fmt.Errorf("failed to send message")
	}

	now := time.Now().Unix()
	if a.agentDB != nil {
		_ = a.agentDB.SaveMessage(phone, "outbound", text, "user", "", 0, now)
		_ = a.agentDB.UpsertConversation(phone, phone, "active", now, now)
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "agent:message", map[string]any{
			"phone":     phone,
			"direction": "outbound",
			"content":   text,
			"timestamp": now,
		})
	}
	return nil
}

// AgentSetConversationStatus updates a conversation's status.
func (a *App) AgentSetConversationStatus(phone, status string) error {
	if a.agentDB == nil {
		return fmt.Errorf("agent db not initialized")
	}
	now := time.Now().Unix()
	return a.agentDB.UpsertConversation(phone, phone, status, now, now)
}

// === Agent Chat ===

// AgentChat sends a message directly to the agent (not through WhatsApp).
func (a *App) AgentChat(message string) (string, error) {
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	if a.agentSvc == nil || a.agentSvc.agent == nil {
		return "", fmt.Errorf("agent not running — start the agent first")
	}
	return a.agentSvc.agent.Chat(a.ctx, message)
}

// === Knowledge Base ===

// AgentUploadKnowledge uploads a document to the knowledge base.
func (a *App) AgentUploadKnowledge(path string) (map[string]any, error) {
	if a.agentSvc == nil || a.agentSvc.rag == nil {
		return nil, fmt.Errorf("agent not started — start the agent first to use knowledge base")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	filename := filepath.Base(path)
	docID, chunkCount, err := a.agentSvc.rag.IngestDocument(a.ctx, filename, data)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":          docID,
		"filename":    filename,
		"chunk_count": chunkCount,
		"status":      "indexed",
	}, nil
}

// AgentListKnowledge returns all indexed documents.
func (a *App) AgentListKnowledge() ([]map[string]any, error) {
	if a.agentSvc == nil || a.agentSvc.rag == nil {
		return nil, fmt.Errorf("agent not started")
	}
	return a.agentSvc.rag.ListDocuments()
}

// AgentDeleteKnowledge removes a document from the knowledge base.
func (a *App) AgentDeleteKnowledge(docID string) error {
	if a.agentSvc == nil || a.agentSvc.rag == nil {
		return fmt.Errorf("agent not started")
	}
	return a.agentSvc.rag.DeleteDocument(docID)
}

// === Statistics ===

// AgentStats returns dashboard statistics.
func (a *App) AgentStats() (map[string]any, error) {
	if a.agentDB == nil {
		return nil, fmt.Errorf("agent db not initialized")
	}
	return a.agentDB.GetStatsSummary()
}

// === Internal ===

func (svc *AgentService) start(config agent.AgentConfig) error {
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return err
	}

	svc.config = config

	// Create the RAG service with embedder.
	embedder := rag.NewEmbedder(config.DeepSeekAPIKey, config.DeepSeekBaseURL, config.EmbeddingModel)
	svc.rag = rag.NewRAG(svc.db.db, embedder, rag.RAGConfig{
		ChunkSize:    config.ChunkSize,
		ChunkOverlap: config.ChunkOverlap,
		TopK:         config.TopK,
	})

	// Create the core Agent with all dependencies.
	sender := svc.app.service.Sender()
	eventHandler := func(eventType string, data map[string]any) {
		if svc.app.ctx != nil {
			runtime.EventsEmit(svc.app.ctx, eventType, data)
		}
	}

	svc.agent = agent.NewAgent(config, svc.db, sender, eventHandler)
	svc.agent.SetRAGSearcher(svc.rag)
	svc.agent.SetScraperService(&scraperAdapter{scraper: svc.app.scraper})
	svc.running = true

	// Create and start the WhatsApp message listener.
	listener := whatsapp.NewListener(sender)
	listener.OnMessage(svc.handleIncomingMessage)

	interval := time.Duration(config.PollIntervalSeconds) * time.Second
	if err := listener.Start(interval); err != nil {
		return fmt.Errorf("start listener: %w", err)
	}

	svc.app.service.SetAgentListener(listener)

	if svc.app.ctx != nil {
		runtime.EventsEmit(svc.app.ctx, "agent:status", map[string]any{
			"running": true,
		})
	}

	log.Printf("[agent] started with model=%s poll_interval=%ds", config.Model, config.PollIntervalSeconds)
	return nil
}

func (svc *AgentService) stop() error {
	svc.running = false

	svc.app.service.SetAgentListener(nil)

	if svc.app.ctx != nil {
		runtime.EventsEmit(svc.app.ctx, "agent:status", map[string]any{
			"status": "stopped",
		})
	}

	log.Printf("[agent] stopped")
	return nil
}

func (svc *AgentService) status() map[string]any {
	return map[string]any{
		"running": svc.running,
		"model":   svc.config.Model,
	}
}

// handleIncomingMessage is called by the WhatsApp listener for each new message.
func (svc *AgentService) handleIncomingMessage(msg whatsapp.IncomingMessage) {
	log.Printf("[agent] incoming message from %s: %q", msg.Phone, msg.Text)

	if svc.agent == nil {
		log.Printf("[agent] agent not initialized, skipping message from %s", msg.Phone)
		return
	}

	// Process message in a goroutine to avoid blocking the listener.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		svc.agent.HandleIncomingMessage(ctx, msg)
	}()
}
