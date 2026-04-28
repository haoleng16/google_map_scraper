package geodata

import "testing"

func TestResolveCountryUnitedKingdom(t *testing.T) {
	country, ok := ResolveCountry("英国")
	if !ok {
		t.Fatal("expected country to resolve")
	}

	if country.DisplayName != "United Kingdom" {
		t.Fatalf("unexpected display name %q", country.DisplayName)
	}

	if len(country.Areas) < 40 {
		t.Fatalf("expected broad city coverage, got %d areas", len(country.Areas))
	}

	if !country.BBox.Contains(51.5074, -0.1278) {
		t.Fatal("expected UK bbox to contain London")
	}
}

func TestResolveCountryJapan(t *testing.T) {
	country, ok := ResolveCountry("日本")
	if !ok {
		t.Fatal("expected country to resolve")
	}

	if country.DisplayName != "Japan" {
		t.Fatalf("unexpected display name %q", country.DisplayName)
	}

	if len(country.Areas) < 30 {
		t.Fatalf("expected broad city coverage, got %d areas", len(country.Areas))
	}

	if !country.BBox.Contains(35.6762, 139.6503) {
		t.Fatal("expected Japan bbox to contain Tokyo")
	}

	if country.BBox.Contains(1.3521, 103.8198) {
		t.Fatal("did not expect Japan bbox to contain Singapore")
	}
}

func TestResolveCountryUnknown(t *testing.T) {
	if _, ok := ResolveCountry("Atlantis"); ok {
		t.Fatal("expected unknown country not to resolve")
	}
}
