package gmaps

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestDocPhoneExtractor(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<a href="tel:+84 912 345 678">Call</a>`))
	if err != nil {
		t.Fatal(err)
	}

	got := docPhoneExtractor(doc)
	if got != "+84 912 345 678" {
		t.Fatalf("expected phone, got %q", got)
	}
}

func TestRegexPhoneExtractor(t *testing.T) {
	got := regexPhoneExtractor([]byte(`Contact us at 0912 345 678 for support.`))
	if got != "0912 345 678" {
		t.Fatalf("expected phone, got %q", got)
	}
}

func TestCleanPhoneRejectsShortValues(t *testing.T) {
	if got := cleanPhone("12345"); got != "" {
		t.Fatalf("expected short value to be rejected, got %q", got)
	}
}
