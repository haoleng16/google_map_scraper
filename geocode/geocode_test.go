package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "New Delhi" {
			t.Fatalf("expected query New Delhi, got %q", got)
		}

		_, _ = w.Write([]byte(`[{
			"display_name": "New Delhi, Delhi, India",
			"boundingbox": ["28.404", "28.883", "76.838", "77.347"],
			"type": "administrative",
			"addresstype": "city",
			"address": {"country_code": "in"}
		}]`))
	}))
	defer srv.Close()

	result, err := resolve(context.Background(), srv.Client(), srv.URL, "New Delhi")
	if err != nil {
		t.Fatal(err)
	}

	if result.DisplayName != "New Delhi, Delhi, India" {
		t.Fatalf("unexpected display name %q", result.DisplayName)
	}

	if result.BoundingBox.MinLat != 28.404 || result.BoundingBox.MaxLon != 77.347 {
		t.Fatalf("unexpected bounding box: %+v", result.BoundingBox)
	}

	if result.CountryCode != "IN" || result.IsCountry {
		t.Fatalf("unexpected country metadata: %+v", result)
	}
}

func TestResolveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, err := resolve(context.Background(), srv.Client(), srv.URL, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveKnownLocation(t *testing.T) {
	result, err := Resolve(context.Background(), "深圳")
	if err != nil {
		t.Fatal(err)
	}

	if result.DisplayName != "深圳市, 广东省, 中国" {
		t.Fatalf("unexpected display name %q", result.DisplayName)
	}
}

func TestResolveKnownForeignLocationInChinese(t *testing.T) {
	result, err := Resolve(context.Background(), "英国")
	if err != nil {
		t.Fatal(err)
	}

	if result.DisplayName != "United Kingdom" {
		t.Fatalf("unexpected display name %q", result.DisplayName)
	}
}

func TestResolveKnownJapanInChinese(t *testing.T) {
	result, err := Resolve(context.Background(), "日本")
	if err != nil {
		t.Fatal(err)
	}

	if result.DisplayName != "Japan" {
		t.Fatalf("unexpected display name %q", result.DisplayName)
	}

	if result.BoundingBox.Contains(1.3521, 103.8198) {
		t.Fatal("did not expect Japan bbox to contain Singapore")
	}
}

func TestResolveCountryMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{
			"display_name": "Germany",
			"boundingbox": ["47.270", "55.099", "5.866", "15.041"],
			"type": "administrative",
			"addresstype": "country",
			"address": {"country_code": "de"}
		}]`))
	}))
	defer srv.Close()

	result, err := resolve(context.Background(), srv.Client(), srv.URL, "Germany")
	if err != nil {
		t.Fatal(err)
	}

	if result.CountryCode != "DE" || !result.IsCountry {
		t.Fatalf("unexpected country metadata: %+v", result)
	}
}
