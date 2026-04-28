package webrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/scrapemate"
)

func TestCSVWriterFiltersOutsideBoundingBox(t *testing.T) {
	bbox := grid.BoundingBox{
		MinLat: 49,
		MinLon: -9,
		MaxLat: 62,
		MaxLon: 2,
	}
	file, csvPath := tempCSVOutput(t)
	writer := newCSVWriter(file, newPlaceFilter(bbox, "GB", "United Kingdom"))
	in := make(chan scrapemate.Result, 2)

	in <- scrapemate.Result{Data: &gmaps.Entry{Link: "https://uk.example", Title: "UK Shop", Latitude: 51.5, Longtitude: -0.1}}
	in <- scrapemate.Result{Data: &gmaps.Entry{Link: "https://sg.example", Title: "SG Shop", Latitude: 1.3, Longtitude: 103.8}}
	close(in)

	if err := writer.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	out := readCSVOutput(t, csvPath)
	if !strings.Contains(out, "UK Shop") {
		t.Fatalf("expected UK row in output: %s", out)
	}

	if strings.Contains(out, "SG Shop") {
		t.Fatalf("did not expect Singapore row in output: %s", out)
	}
}

func TestCSVWriterFiltersExplicitForeignCountryInsideBoundingBox(t *testing.T) {
	bbox := grid.BoundingBox{
		MinLat: 5.5,
		MinLon: 97.3,
		MaxLat: 20.5,
		MaxLon: 105.8,
	}
	file, csvPath := tempCSVOutput(t)
	writer := newCSVWriter(file, newPlaceFilter(bbox, "TH", "Thailand"))
	in := make(chan scrapemate.Result, 2)

	in <- scrapemate.Result{Data: &gmaps.Entry{
		Link:       "https://th.example",
		Title:      "Bangkok Shop",
		Latitude:   13.7563,
		Longtitude: 100.5018,
		CompleteAddress: gmaps.Address{
			Country: "TH",
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

	out := readCSVOutput(t, csvPath)
	if !strings.Contains(out, "Bangkok Shop") {
		t.Fatalf("expected Thailand row in output: %s", out)
	}

	if strings.Contains(out, "Cambodia Shop") {
		t.Fatalf("did not expect explicit foreign country row in output: %s", out)
	}
}

func TestCSVWriterReplacesSearchCandidateWithDetails(t *testing.T) {
	file, csvPath := tempCSVOutput(t)
	writer := newCSVWriter(file, nil)
	in := make(chan scrapemate.Result, 2)

	link := "https://www.google.com/maps/place/shop/data=!4m7!3d13.7!4d100.5?authuser=0"
	in <- scrapemate.Result{Data: &gmaps.Entry{
		Link:       link,
		Title:      "Candidate Shop",
		Latitude:   13.7,
		Longtitude: 100.5,
	}}
	in <- scrapemate.Result{Data: &gmaps.Entry{
		Link:       link,
		Title:      "Candidate Shop",
		Category:   "Cell phone store",
		Address:    "Bangkok, Thailand",
		Phone:      "+66123456789",
		Latitude:   13.7,
		Longtitude: 100.5,
	}}
	close(in)

	if err := writer.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	out := readCSVOutput(t, csvPath)
	if strings.Count(out, "Candidate Shop") != 1 {
		t.Fatalf("expected one deduplicated row: %s", out)
	}
	if !strings.Contains(out, "Cell phone store") || !strings.Contains(out, "+66123456789") {
		t.Fatalf("expected detailed row to replace candidate row: %s", out)
	}
}

func tempCSVOutput(t *testing.T) (*os.File, string) {
	t.Helper()

	csvPath := filepath.Join(t.TempDir(), "results.csv")
	file, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = file.Close()
	})

	return file, csvPath
}

func readCSVOutput(t *testing.T, csvPath string) string {
	t.Helper()

	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
