package rag

import "strings"

// ChunkText splits text into overlapping chunks of approximately chunkSize characters.
func ChunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}

	// Split into paragraphs first for natural boundaries.
	paragraphs := splitParagraphs(text)

	var chunks []string
	var current strings.Builder
	currentLen := 0

	for _, para := range paragraphs {
		// If adding this paragraph exceeds chunk size, flush current chunk.
		if currentLen > 0 && currentLen+len(para)+1 > chunkSize {
			chunks = append(chunks, current.String())
			// Keep overlap from end of current chunk.
			overlapText := extractOverlap(current.String(), overlap)
			current.Reset()
			current.WriteString(overlapText)
			currentLen = len(overlapText)
		}

		if currentLen > 0 {
			current.WriteString("\n")
			currentLen++
		}
		current.WriteString(para)
		currentLen += len(para)
	}

	if currentLen > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// splitParagraphs splits text on double newlines or single newlines if no doubles exist.
func splitParagraphs(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Normalize line endings.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	var paragraphs []string
	if strings.Contains(text, "\n\n") {
		for _, p := range strings.Split(text, "\n\n") {
			p = strings.TrimSpace(p)
			if p != "" {
				paragraphs = append(paragraphs, p)
			}
		}
	} else {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				paragraphs = append(paragraphs, line)
			}
		}
	}
	return paragraphs
}

// extractOverlap returns the last `overlap` characters from text, split at a word boundary.
func extractOverlap(text string, overlap int) string {
	if len(text) <= overlap {
		return text
	}
	start := len(text) - overlap
	// Try to start at a word boundary.
	if idx := strings.IndexByte(text[start:], ' '); idx > 0 {
		start += idx + 1
	}
	return text[start:]
}
