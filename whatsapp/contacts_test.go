package whatsapp

import (
	"strings"
	"testing"
)

func TestParseContactsSupportsChineseGoogleMapsHeaders(t *testing.T) {
	csvData := `地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网
https://maps.example/place,测试商家,咖啡店,测试地址,{},+84 703 599 367,27,5.0,10.1,106.1,test@example.com,https://example.com
`

	contacts, err := ParseContacts(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("parse contacts: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("len(contacts) = %d, want 1", len(contacts))
	}
	contact := contacts[0]
	if contact.ShopName != "测试商家" {
		t.Fatalf("ShopName = %q, want Chinese header value", contact.ShopName)
	}
	if contact.Phone != "84703599367" {
		t.Fatalf("Phone = %q, want cleaned Chinese header value", contact.Phone)
	}
	if contact.Category != "咖啡店" {
		t.Fatalf("Category = %q, want Chinese header value", contact.Category)
	}
	if contact.Email != "test@example.com" {
		t.Fatalf("Email = %q, want Chinese header value", contact.Email)
	}
	if contact.Website != "https://example.com" {
		t.Fatalf("Website = %q, want Chinese header value", contact.Website)
	}
	if !contact.Selected {
		t.Fatal("Selected = false, want true")
	}
}
