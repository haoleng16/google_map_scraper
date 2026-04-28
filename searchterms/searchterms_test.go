package searchterms

import (
	"slices"
	"testing"
)

func TestExpandAddsEnglishTermsForChineseKeyword(t *testing.T) {
	got := Expand([]string{"手机店"})
	want := []string{"手机店", "mobile phone shop", "cell phone store"}

	if !slices.Equal(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestExpandKeepsEnglishKeyword(t *testing.T) {
	got := Expand([]string{"restaurant"})
	want := []string{"restaurant"}

	if !slices.Equal(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestExpandTranslatesCompoundChineseKeyword(t *testing.T) {
	got := Expand([]string{"附近手机店"})
	want := []string{"附近手机店", "mobile phone shop", "cell phone store"}

	if !slices.Equal(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestExpandForForeignLocationPrefersEnglishTerms(t *testing.T) {
	got := ExpandForLocation([]string{"手机店"}, "United Kingdom")
	want := []string{"mobile phone shop", "cell phone store"}

	if !slices.Equal(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestExpandForChinaKeepsChineseTerms(t *testing.T) {
	got := ExpandForLocation([]string{"手机店"}, "深圳市, 广东省, 中国")
	want := []string{"手机店", "mobile phone shop", "cell phone store"}

	if !slices.Equal(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}
