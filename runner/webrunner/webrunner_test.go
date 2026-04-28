package webrunner

import "testing"

func TestLocationQualifiedQueries(t *testing.T) {
	got := locationQualifiedQueries([]string{"手机店", "mobile phone shop", " restaurant ", ""}, "英国")
	want := "手机店 英国\nmobile phone shop 英国\nrestaurant 英国"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLangForLocation(t *testing.T) {
	if got := langForLocation("zh", "United Kingdom"); got != "en" {
		t.Fatalf("expected en, got %q", got)
	}

	if got := langForLocation("zh", "深圳市, 广东省, 中国"); got != "zh" {
		t.Fatalf("expected zh, got %q", got)
	}
}
