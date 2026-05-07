package rag

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	oai "github.com/sashabaranov/go-openai"
)

// Embedder calls an OpenAI-compatible embedding API to generate vectors.
type Embedder struct {
	client *oai.Client
	model  string
}

// NewEmbedder creates a new embedder with the given API credentials.
func NewEmbedder(apiKey, baseURL, model string) *Embedder {
	config := oai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	if model == "" {
		model = "text-embedding-v3"
	}
	return &Embedder{
		client: oai.NewClientWithConfig(config),
		model:  model,
	}
}

// Embed generates an embedding vector for the given text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := e.client.CreateEmbeddings(ctx, oai.EmbeddingRequestStrings{
		Input: []string{text},
		Model: oai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("embedding API call: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no data")
	}

	return resp.Data[0].Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts in a single API call.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	const batchSize = 100
	var results [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		resp, err := e.client.CreateEmbeddings(ctx, oai.EmbeddingRequestStrings{
			Input: batch,
			Model: oai.EmbeddingModel(e.model),
		})
		if err != nil {
			return nil, fmt.Errorf("embedding batch %d: %w", i/batchSize, err)
		}

		for _, d := range resp.Data {
			results = append(results, d.Embedding)
		}
	}

	return results, nil
}

// EncodeVector serializes a float32 slice to bytes for SQLite BLOB storage.
func EncodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DecodeVector deserializes bytes from SQLite BLOB to float32 slice.
func DecodeVector(data []byte) []float32 {
	n := len(data) / 4
	v := make([]float32, n)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return v
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// SearchResult holds a chunk matched by vector search.
type SearchResult struct {
	Content string
	Score   float32
	ChunkID int64
	DocID   string
}

// RankBySimilarity sorts candidates by cosine similarity to the query vector
// and returns the top K results.
func RankBySimilarity(query []float32, candidates []SearchResult, topK int) []SearchResult {
	for i := range candidates {
		candidates[i].Score = CosineSimilarity(query, DecodeVector([]byte(candidates[i].Content)))
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates
}
