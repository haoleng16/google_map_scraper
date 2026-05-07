package whatsapp

import (
	"strings"
	"testing"
)

func TestAttachDocumentSelectorsSupportChineseMenu(t *testing.T) {
	selectors := strings.Join(Selectors["attach_document_option"], "\n")

	if !strings.Contains(selectors, "文档") {
		t.Fatal("attach document selectors do not include the Chinese WhatsApp document label")
	}
}

func TestLoggedInSelectorsCoverChatListWithoutOpenChat(t *testing.T) {
	selectors := strings.Join(Selectors["logged_in"], "\n")

	for _, expected := range []string{"#side", "Chat list", "聊天列表"} {
		if !strings.Contains(selectors, expected) {
			t.Fatalf("logged in selectors do not include %q", expected)
		}
	}
}

func TestInvalidPhonePopupSelectorsCoverChineseAndEnglishPrompts(t *testing.T) {
	selectors := strings.Join(Selectors["invalid_phone_popup"], "\n")

	for _, expected := range []string{"未注册", "无效", "not registered", "invalid"} {
		if !strings.Contains(selectors, expected) {
			t.Fatalf("invalid phone popup selectors do not include %q", expected)
		}
	}
}

func TestDismissPopupSelectorsCoverChineseConfirmButtons(t *testing.T) {
	selectors := strings.Join(Selectors["dismiss_popup"], "\n")

	for _, expected := range []string{"好的", "确定", "OK"} {
		if !strings.Contains(selectors, expected) {
			t.Fatalf("dismiss popup selectors do not include %q", expected)
		}
	}
}
