package agent

import "strings"

// blockedPatterns contains keywords that should prevent auto-reply.
// These cover sensitive topics where AI hallucination could cause harm.
var blockedPatterns = []string{
	"转账", "汇款", "付款", "银行卡号", "密码",
	"政治", "宗教", "赌博",
	"self-harm", "suicide", "kill",
	"hack", "exploit", "malware",
}

// BlockReason describes why a message was blocked.
type BlockReason struct {
	Blocked bool
	Reason  string
}

// ShouldBlock checks if the content contains sensitive patterns that should
// prevent automatic sending. Returns a BlockReason with details.
func ShouldBlock(content string) BlockReason {
	lower := strings.ToLower(content)

	for _, pattern := range blockedPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return BlockReason{
				Blocked: true,
				Reason:  "content contains sensitive keyword: " + pattern,
			}
		}
	}

	// Block very long responses that may indicate hallucination.
	if len([]rune(content)) > 500 {
		return BlockReason{
			Blocked: true,
			Reason:  "response exceeds 500 characters, possible hallucination",
		}
	}

	return BlockReason{Blocked: false}
}

// SanitizeReply trims and normalizes a reply before sending.
func SanitizeReply(content string) string {
	content = strings.TrimSpace(content)

	// Remove wrapping quotes the LLM might add.
	if len(content) >= 2 {
		if (content[0] == '"' && content[len(content)-1] == '"') ||
			(content[0] == '\'' && content[len(content)-1] == '\'') ||
			(content[0] == '`' && content[len(content)-1] == '`') {
			content = content[1 : len(content)-1]
		}
	}

	return content
}
