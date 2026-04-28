package webrunner

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/scrapemate"
)

func TestCSVWriterFiltersOutsideBoundingBox(t *testing.T) {
	var buf bytes.Buffer
	bbox := grid.BoundingBox{
		MinLat: 49,
		MinLon: -9,
		MaxLat: 62,
		MaxLon: 2,
	}
	writer := newCSVWriter(csv.NewWriter(&buf), newPlaceFilter(bbox, "GB", "United Kingdom"))
	in := make(chan scrapemate.Result, 2)

	in <- scrapemate.Result{Data: &gmaps.Entry{Link: "https://uk.example", Title: "UK Shop", Latitude: 51.5, Longtitude: -0.1}}
	in <- scrapemate.Result{Data: &gmaps.Entry{Link: "https://sg.example", Title: "SG Shop", Latitude: 1.3, Longtitude: 103.8}}
	close(in)

	if err := writer.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "UK Shop") {
		t.Fatalf("expected UK row in output: %s", out)
	}

	if strings.Contains(out, "SG Shop") {
		t.Fatalf("did not expect Singapore row in output: %s", out)
	}
}

func TestCSVWriterFiltersExplicitForeignCountryInsideBoundingBox(t *testing.T) {
	var buf bytes.Buffer
	bbox := grid.BoundingBox{
		MinLat: 5.5,
		MinLon: 97.3,
		MaxLat: 20.5,
		MaxLon: 105.8,
	}
	writer := newCSVWriter(csv.NewWriter(&buf), newPlaceFilter(bbox, "TH", "Thailand"))
	in := make(chan scrapemate.Result, 2)

	in <- scrapemate.Result{Data: &gmaps.Entry{
		Link:       "https://th.example",
		Title:      "Bangkok Shop",
		Latitude:   13.7563,
		Longtitude: 100.5018,
		CompleteAddress: gmaps.Address{
			Country: "Thailand",
		},
	}}
	in <- scrapemate.Result{Data: &gmaps.Entry{
		Link:       "https://kh.example",
		Title:      "Cambodia Shop",
		Latitude:   13.3671,
		Longtitude: 103.8448,
		CompleteAddress: gmaps.Address{
			Country: "Cambodia",
		},
	}}
	close(in)

	if err := writer.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Bangkok Shop") {
		t.Fatalf("expected Thailand row in output: %s", out)
	}

	if strings.Contains(out, "Cambodia Shop") {
		t.Fatalf("did not expect explicit foreign country row in output: %s", out)
	}
}
