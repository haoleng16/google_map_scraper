package gmaps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosom/scrapemate"
)

func TestGmapJobUsesSearchCandidatesInResults(t *testing.T) {
	job := NewGmapJob("id", "en", "mobile phone shop Bangkok, Thailand", 10, false, "13.7,100.5", 15)

	if !job.UseInResults() {
		t.Fatal("expected search candidates to be written to results")
	}
}

func TestCoordinatesFromMapsURL(t *testing.T) {
	lat, lon, ok := coordinatesFromMapsURL("https://www.google.com/maps/place/shop/data=!4m7!3d13.7467291!4d100.539628!16s")
	if !ok {
		t.Fatal("expected coordinates")
	}

	if lat != 13.7467291 || lon != 100.539628 {
		t.Fatalf("unexpected coordinates: %f,%f", lat, lon)
	}
}

func TestGmapJobChainsSearchResultsOnePlaceAtATime(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div role="feed">
			<div jsaction><a aria-label="Shop 1" href="https://www.google.com/maps/place/shop-1/data=!4m7!3d13.1!4d100.1!16s"></a></div>
			<div jsaction><a aria-label="Shop 2" href="https://www.google.com/maps/place/shop-2/data=!4m7!3d13.2!4d100.2!16s"></a></div>
			<div jsaction><a aria-label="Shop 3" href="https://www.google.com/maps/place/shop-3/data=!4m7!3d13.3!4d100.3!16s"></a></div>
		</div>
	`))
	if err != nil {
		t.Fatal(err)
	}

	job := NewGmapJob("id", "en", "mobile phone shop Bangkok, Thailand", 10, true, "13.7,100.5", 15)
	data, next, err := job.Process(context.Background(), &scrapemate.Response{
		URL:      "https://www.google.com/maps/search/mobile+phone+shop",
		Document: doc,
	})
	if err != nil {
		t.Fatal(err)
	}

	candidates, ok := data.([]*Entry)
	if !ok {
		t.Fatalf("expected search candidates, got %T", data)
	}
	if len(candidates) != 1 || candidates[0].Title != "Shop 1" {
		t.Fatalf("expected only first candidate, got %#v", candidates)
	}
	if len(next) != 1 {
		t.Fatalf("expected one next place job, got %d", len(next))
	}

	placeJob, ok := next[0].(*PlaceJob)
	if !ok {
		t.Fatalf("expected place job, got %T", next[0])
	}
	if placeJob.URL != "https://www.google.com/maps/place/shop-1/data=!4m7!3d13.1!4d100.1!16s" {
		t.Fatalf("unexpected first place url: %q", placeJob.URL)
	}
	if len(placeJob.NextPlaces) != 2 {
		t.Fatalf("expected 2 queued follow-up places, got %d", len(placeJob.NextPlaces))
	}
}

func TestPlaceJobContinuesChainWhenDetailFails(t *testing.T) {
	candidate := &Entry{Title: "Shop 1", Link: "https://example.com/shop-1"}
	job := NewPlaceJob("root", "en", "https://www.google.com/maps/place/shop-1", true, false)
	job.SearchCandidate = candidate
	job.NextPlaces = []placeFollowup{
		{URL: "https://www.google.com/maps/place/shop-2", Candidate: &Entry{Title: "Shop 2"}},
	}

	data, next, err := job.Process(context.Background(), &scrapemate.Response{Error: errors.New("detail failed")})
	if err != nil {
		t.Fatal(err)
	}
	if data != candidate {
		t.Fatalf("expected failed detail to return its candidate")
	}
	if len(next) != 1 {
		t.Fatalf("expected next place to continue chain, got %d", len(next))
	}
}

func TestEmailJobContinuesChainAfterWebsiteFetch(t *testing.T) {
	entry := &Entry{Title: "Shop 1", WebSite: "https://example.com"}
	job := NewEmailJob("place-1", entry)
	job.RootParentID = "root"
	job.LangCode = "en"
	job.ExtractEmail = true
	job.NextPlaces = []placeFollowup{
		{URL: "https://www.google.com/maps/place/shop-2", Candidate: &Entry{Title: "Shop 2"}},
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<a href="mailto:sales@example.com">Email</a>`))
	if err != nil {
		t.Fatal(err)
	}

	data, next, err := job.Process(context.Background(), &scrapemate.Response{Document: doc})
	if err != nil {
		t.Fatal(err)
	}
	if data != entry {
		t.Fatalf("expected email job to return original entry")
	}
	if len(entry.Emails) != 1 || entry.Emails[0] != "sales@example.com" {
		t.Fatalf("expected extracted email, got %#v", entry.Emails)
	}
	if len(next) != 1 {
		t.Fatalf("expected next place to continue chain, got %d", len(next))
	}
}
