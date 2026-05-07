package rag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// RAG provides document ingestion and semantic search over a knowledge base.
type RAG struct {
	db       *sql.DB
	embedder *Embedder
	config   RAGConfig
}

// RAGConfig holds configuration for the RAG service.
type RAGConfig struct {
	ChunkSize    int
	ChunkOverlap int
	TopK         int
}

// NewRAG creates a new RAG service backed by the given SQLite database.
func NewRAG(db *sql.DB, embedder *Embedder, config RAGConfig) *RAG {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 500
	}
	if config.ChunkOverlap < 0 {
		config.ChunkOverlap = 0
	}
	if config.TopK <= 0 {
		config.TopK = 5
	}
	return &RAG{db: db, embedder: embedder, config: config}
}

// IngestDocument parses a document, chunks it, generates embeddings, and stores everything.
func (r *RAG) IngestDocument(ctx context.Context, filename string, data []byte) (docID string, chunkCount int, err error) {
	text, err := r.parseDocument(filename, data)
	if err != nil {
		return "", 0, fmt.Errorf("parse document: %w", err)
	}

	hash := sha256.Sum256([]byte(text))
	contentHash := fmt.Sprintf("%x", hash[:16])

	existingID := r.findDocByHash(contentHash)
	if existingID != "" {
		return existingID, 0, fmt.Errorf("document with same content already exists (id: %s)", existingID)
	}

	chunks := ChunkText(text, r.config.ChunkSize, r.config.ChunkOverlap)
	if len(chunks) == 0 {
		return "", 0, fmt.Errorf("no text content to index")
	}

	embeddings, err := r.embedder.EmbedBatch(ctx, chunks)
	if err != nil {
		return "", 0, fmt.Errorf("generate embeddings: %w", err)
	}

	docID = fmt.Sprintf("doc_%d", time.Now().UnixNano())
	sourceType := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	now := time.Now().Unix()

	_, err = r.db.Exec(
		`INSERT INTO kb_documents (id, filename, source_type, content_hash, chunk_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		docID, filename, sourceType, contentHash, len(chunks), now, now,
	)
	if err != nil {
		return "", 0, fmt.Errorf("insert document: %w", err)
	}

	for i, chunk := range chunks {
		embedding := EncodeVector(embeddings[i])
		_, err = r.db.Exec(
			`INSERT INTO kb_chunks (document_id, chunk_index, content, embedding, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			docID, i, chunk, embedding, now,
		)
		if err != nil {
			return docID, i, fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}

	log.Printf("[rag] ingested %s: %d chunks, %d bytes", filename, len(chunks), len(text))
	return docID, len(chunks), nil
}

// Search finds the most relevant chunks for the given query text.
func (r *RAG) Search(ctx context.Context, queryText string) ([]string, error) {
	queryVec, err := r.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	rows, err := r.db.Query(
		`SELECT c.id, c.document_id, c.content, c.embedding FROM kb_chunks c`,
	)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		content string
		score   float32
	}
	var candidates []candidate

	for rows.Next() {
		var id int64
		var docID, content string
		var embeddingBytes []byte
		if err := rows.Scan(&id, &docID, &content, &embeddingBytes); err != nil {
			continue
		}
		vec := DecodeVector(embeddingBytes)
		score := CosineSimilarity(queryVec, vec)
		candidates = append(candidates, candidate{content: content, score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	topK := r.config.TopK
	if topK > len(candidates) {
		topK = len(candidates)
	}

	var results []string
	for i := 0; i < topK; i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[best].score {
				best = j
			}
		}
		candidates[i], candidates[best] = candidates[best], candidates[i]
		results = append(results, candidates[i].content)
	}

	return results, nil
}

// ListDocuments returns metadata for all indexed documents.
func (r *RAG) ListDocuments() ([]map[string]any, error) {
	rows, err := r.db.Query(
		`SELECT id, filename, source_type, content_hash, chunk_count, created_at, updated_at
		 FROM kb_documents ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []map[string]any
	for rows.Next() {
		var id, filename, sourceType, contentHash string
		var chunkCount int
		var createdAt, updatedAt int64
		if err := rows.Scan(&id, &filename, &sourceType, &contentHash, &chunkCount, &createdAt, &updatedAt); err != nil {
			continue
		}
		docs = append(docs, map[string]any{
			"id":           id,
			"filename":     filename,
			"source_type":  sourceType,
			"content_hash": contentHash,
			"chunk_count":  chunkCount,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}
	return docs, rows.Err()
}

// DeleteDocument removes a document and all its chunks from the knowledge base.
func (r *RAG) DeleteDocument(docID string) error {
	_, err := r.db.Exec(`DELETE FROM kb_documents WHERE id = ?`, docID)
	return err
}

func (r *RAG) parseDocument(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return ParsePDF(data)
	case ".docx", ".doc":
		return ParseDOCX(data)
	case ".txt", ".md", ".csv":
		return ParseTXT(data)
	default:
		return "", fmt.Errorf("unsupported file type: %s (supported: .pdf, .docx, .txt, .md, .csv)", ext)
	}
}

func (r *RAG) findDocByHash(hash string) string {
	var id string
	err := r.db.QueryRow(`SELECT id FROM kb_documents WHERE content_hash = ?`, hash).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}
