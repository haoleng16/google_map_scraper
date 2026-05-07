package rag

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/nguyenthenguyen/docx"
)

// ParseDOCX extracts text content from a .docx file.
func ParseDOCX(data []byte) (string, error) {
	doc, err := docx.ReadDocxFromMemory(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	defer doc.Close()

	text := strings.TrimSpace(doc.Editable().GetContent())

	if text == "" {
		return "", fmt.Errorf("no text extracted from DOCX")
	}
	return text, nil
}
