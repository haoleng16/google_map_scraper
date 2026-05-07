package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// agentDB wraps the SQLite connection for the agent subsystem.
type agentDB struct {
	db *sql.DB
}

// newAgentDB opens (or creates) the agent database at the given path.
func newAgentDB(dataFolder string) (*agentDB, error) {
	dbPath := filepath.Join(dataFolder, "agent", "agent.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create agent db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open agent db: %w", err)
	}

	if err := runAgentMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("agent db migrations: %w", err)
	}

	return &agentDB{db: db}, nil
}

func (a *agentDB) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	return a.db.Close()
}

func runAgentMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS conversations (
		phone       TEXT PRIMARY KEY,
		name        TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'active',
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS messages (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		phone       TEXT NOT NULL,
		direction   TEXT NOT NULL,
		content     TEXT NOT NULL,
		role        TEXT NOT NULL,
		tool_name   TEXT NOT NULL DEFAULT '',
		tokens_used INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_messages_phone ON messages(phone, created_at);

	CREATE TABLE IF NOT EXISTS kb_documents (
		id           TEXT PRIMARY KEY,
		filename     TEXT NOT NULL,
		source_type  TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		chunk_count  INTEGER NOT NULL DEFAULT 0,
		created_at   INTEGER NOT NULL,
		updated_at   INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS kb_chunks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		document_id TEXT NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
		chunk_index INTEGER NOT NULL,
		content     TEXT NOT NULL,
		embedding   BLOB NOT NULL,
		created_at  INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_chunks_doc ON kb_chunks(document_id);

	CREATE TABLE IF NOT EXISTS agent_stats (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		event_type  TEXT NOT NULL,
		phone       TEXT NOT NULL DEFAULT '',
		detail      TEXT NOT NULL DEFAULT '',
		tokens_used INTEGER NOT NULL DEFAULT 0,
		latency_ms  INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_stats_created ON agent_stats(event_type, created_at);
	`
	_, err := db.Exec(schema)
	return err
}

// SaveMessage persists a message to the messages table.
func (a *agentDB) SaveMessage(phone, direction, content, role, toolName string, tokensUsed, createdAt int64) error {
	_, err := a.db.Exec(
		`INSERT INTO messages (phone, direction, content, role, tool_name, tokens_used, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		phone, direction, content, role, toolName, tokensUsed, createdAt,
	)
	return err
}

// GetMessages returns the most recent messages for a phone number.
func (a *agentDB) GetMessages(phone string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := a.db.Query(
		`SELECT id, phone, direction, content, role, tool_name, tokens_used, created_at
		 FROM messages WHERE phone = ? ORDER BY created_at DESC LIMIT ?`,
		phone, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]any
	for rows.Next() {
		var id int64
		var pPhone, direction, content, role, toolName string
		var tokensUsed, createdAt int64
		if err := rows.Scan(&id, &pPhone, &direction, &content, &role, &toolName, &tokensUsed, &createdAt); err != nil {
			return nil, err
		}
		messages = append(messages, map[string]any{
			"id":          id,
			"phone":       pPhone,
			"direction":   direction,
			"content":     content,
			"role":        role,
			"tool_name":   toolName,
			"tokens_used": tokensUsed,
			"created_at":  createdAt,
		})
	}
	return messages, rows.Err()
}

// UpsertConversation creates or updates a conversation record.
func (a *agentDB) UpsertConversation(phone, name, status string, createdAt, updatedAt int64) error {
	_, err := a.db.Exec(
		`INSERT INTO conversations (phone, name, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(phone) DO UPDATE SET name=excluded.name, status=excluded.status, updated_at=excluded.updated_at`,
		phone, name, status, createdAt, updatedAt,
	)
	return err
}

// GetConversations returns all conversations ordered by most recent update.
func (a *agentDB) GetConversations() ([]map[string]any, error) {
	rows, err := a.db.Query(
		`SELECT phone, name, status, created_at, updated_at
		 FROM conversations ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []map[string]any
	for rows.Next() {
		var phone, name, status string
		var createdAt, updatedAt int64
		if err := rows.Scan(&phone, &name, &status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		conversations = append(conversations, map[string]any{
			"phone":      phone,
			"name":       name,
			"status":     status,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}
	return conversations, rows.Err()
}

// RecordStat inserts an agent stats event.
func (a *agentDB) RecordStat(eventType, phone, detail string, tokensUsed, latencyMs, createdAt int64) error {
	_, err := a.db.Exec(
		`INSERT INTO agent_stats (event_type, phone, detail, tokens_used, latency_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		eventType, phone, detail, tokensUsed, latencyMs, createdAt,
	)
	return err
}

// GetStatsSummary returns aggregated stats for today.
func (a *agentDB) GetStatsSummary() (map[string]any, error) {
	row := a.db.QueryRow(`
		SELECT
			COUNT(CASE WHEN event_type='message_received' THEN 1 END) AS messages_received,
			COUNT(CASE WHEN event_type='reply_sent' THEN 1 END) AS replies_sent,
			COUNT(CASE WHEN event_type='tool_call' THEN 1 END) AS tool_calls,
			COUNT(CASE WHEN event_type='error' THEN 1 END) AS errors,
			COALESCE(SUM(tokens_used), 0) AS total_tokens,
			COALESCE(AVG(CASE WHEN latency_ms > 0 THEN latency_ms END), 0) AS avg_latency_ms
		FROM agent_stats
		WHERE created_at >= strftime('%s', 'now', 'start of day')
	`)
	var received, sent, toolCalls, errors, totalTokens int64
	var avgLatency float64
	if err := row.Scan(&received, &sent, &toolCalls, &errors, &totalTokens, &avgLatency); err != nil {
		return nil, err
	}
	return map[string]any{
		"messages_received": received,
		"replies_sent":      sent,
		"tool_calls":        toolCalls,
		"errors":            errors,
		"total_tokens":      totalTokens,
		"avg_latency_ms":    avgLatency,
	}, nil
}
