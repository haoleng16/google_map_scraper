package rag

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// ParsePDF extracts text content from a PDF file.
func ParsePDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	plainText, err := reader.GetPlainText()
	if err != nil {
		return parsePDFPages(reader)
	}
	textData, err := io.ReadAll(plainText)
	if err != nil {
		return "", fmt.Errorf("read pdf text: %w", err)
	}

	return string(textData), nil
}

func parsePDFPages(reader *pdf.Reader) (string, error) {
	var text string
	n := reader.NumPage()
	for i := 1; i <= n; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text += content + "\n"
	}
	if text == "" {
		return "", fmt.Errorf("no text extracted from PDF")
	}
	return text, nil
}
