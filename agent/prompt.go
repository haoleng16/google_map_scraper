package agent

import (
	"fmt"
	"strings"
)

// BuildSystemPrompt constructs the system prompt for the agent.
func BuildSystemPrompt(agentName string, ragContext string) string {
	var b strings.Builder

	b.WriteString("你是 ")
	b.WriteString(agentName)
	b.WriteString("，一个专业的 WhatsApp 客服助手。\n\n")

	b.WriteString("## 核心规则\n")
	b.WriteString("1. 用客户的语言回复（客户用中文就用中文，用英文就用英文）\n")
	b.WriteString("2. 保持回复简洁、专业、友好\n")
	b.WriteString("3. 不要编造不确定的信息，诚实说明你需要查证\n")
	b.WriteString("4. 不要提及你是 AI 或语言模型\n")
	b.WriteString("5. 如果客户问到不在知识库中的问题，建议他们联系人工客服\n")
	b.WriteString("6. 回复控制在 200 字以内，除非需要详细说明\n\n")

	if ragContext != "" {
		b.WriteString("## 公司知识库\n")
		b.WriteString("以下是从公司文档中检索到的相关信息，请基于这些信息回答客户问题：\n\n")
		b.WriteString(ragContext)
		b.WriteString("\n\n")
	}

	b.WriteString("## 可用工具\n")
	b.WriteString("- search_maps: 搜索 Google Maps 上的商家信息（需要提供关键词和地点）\n")
	b.WriteString("- send_message: 向客户发送消息（仅在你需要主动通知客户时使用）\n\n")

	b.WriteString("## 回复格式\n")
	b.WriteString("直接回复客户，不需要前缀或标记。如果需要使用工具，请自然地告知客户你正在查找信息。\n")

	return b.String()
}

// BuildRAGContext formats retrieved RAG chunks into context for the prompt.
func BuildRAGContext(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	for i, chunk := range chunks {
		b.WriteString("[文档片段 ")
		b.WriteString(intToOrdinal(i + 1))
		b.WriteString("]\n")
		b.WriteString(chunk)
		b.WriteString("\n\n")
	}
	return b.String()
}

func intToOrdinal(n int) string {
	s := fmt.Sprintf("%d", n)
	switch n % 10 {
	case 1:
		if n%100 != 11 {
			return s + "st"
		}
	case 2:
		if n%100 != 12 {
			return s + "nd"
		}
	case 3:
		if n%100 != 13 {
			return s + "rd"
		}
	}
	return s + "th"
}
