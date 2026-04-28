package gmaps

import (
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/scrapemate"
)

type placeFollowup struct {
	URL       string
	Candidate *Entry
}

type placeJobConfig struct {
	parentID            string
	langCode            string
	extractEmail        bool
	extractExtraReviews bool
	exitMonitor         exiter.Exiter
	writerManaged       bool
}

func newPlaceJobConfig(parentID, langCode string, extractEmail, extractExtraReviews bool) placeJobConfig {
	return placeJobConfig{
		parentID:            parentID,
		langCode:            langCode,
		extractEmail:        extractEmail,
		extractExtraReviews: extractExtraReviews,
	}
}

func newPlaceJobChain(cfg placeJobConfig, followups []placeFollowup) (*Entry, scrapemate.IJob) {
	if len(followups) == 0 {
		return nil, nil
	}

	first := followups[0]
	opts := []PlaceJobOptions{}

	if cfg.exitMonitor != nil {
		opts = append(opts, WithPlaceJobExitMonitor(cfg.exitMonitor))
	}

	if cfg.writerManaged {
		opts = append(opts, WithPlaceJobWriterManagedCompletion())
	}

	job := NewPlaceJob(
		cfg.parentID,
		cfg.langCode,
		first.URL,
		cfg.extractEmail,
		cfg.extractExtraReviews,
		opts...,
	)
	job.SearchCandidate = first.Candidate
	job.NextPlaces = append([]placeFollowup(nil), followups[1:]...)

	return first.Candidate, job
}
