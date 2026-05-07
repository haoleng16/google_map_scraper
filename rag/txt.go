package rag

// ParseTXT returns the text content as-is (no parsing needed).
func ParseTXT(data []byte) (string, error) {
	return string(data), nil
}
