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
	filter := grid.BoundingBox{
		MinLat: 49,
		MinLon: -9,
		MaxLat: 62,
		MaxLon: 2,
	}
	writer := newCSVWriter(csv.NewWriter(&buf), &filter)
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
