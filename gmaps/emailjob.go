package gmaps

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gosom/scrapemate"
	"github.com/mcnijman/go-emailaddress"

	"github.com/gosom/google-maps-scraper/exiter"
)

var phoneNumberRe = regexp.MustCompile(`(?:\+?\d[\d\s().-]{7,}\d)`)

type EmailExtractJobOptions func(*EmailExtractJob)

type EmailExtractJob struct {
	scrapemate.Job

	Entry                   *Entry
	RootParentID            string
	LangCode                string
	ExtractEmail            bool
	ExtractExtraReviews     bool
	NextPlaces              []placeFollowup
	ExitMonitor             exiter.Exiter
	WriterManagedCompletion bool
}

func NewEmailJob(parentID string, entry *Entry, opts ...EmailExtractJobOptions) *EmailExtractJob {
	const (
		defaultPrio       = scrapemate.PriorityHigh
		defaultMaxRetries = 0
	)

	job := EmailExtractJob{
		Job: scrapemate.Job{
			ID:         uuid.New().String(),
			ParentID:   parentID,
			Method:     "GET",
			URL:        normalizeGoogleURL(entry.WebSite),
			MaxRetries: defaultMaxRetries,
			Priority:   defaultPrio,
		},
	}

	job.Entry = entry

	for _, opt := range opts {
		opt(&job)
	}

	return &job
}

func WithEmailJobExitMonitor(exitMonitor exiter.Exiter) EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.ExitMonitor = exitMonitor
	}
}

func WithEmailJobWriterManagedCompletion() EmailExtractJobOptions {
	return func(j *EmailExtractJob) {
		j.WriterManagedCompletion = true
	}
}

func (j *EmailExtractJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
	}()

	defer func() {
		if j.ExitMonitor != nil && !j.WriterManagedCompletion {
			j.ExitMonitor.IncrPlacesCompleted(1)
		}
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	log.Info("Processing email job", "url", j.URL)

	// if html fetch failed just return
	if resp.Error != nil {
		return j.Entry, j.nextPlaceJobs(), nil
	}

	doc, ok := resp.Document.(*goquery.Document)
	if !ok {
		return j.Entry, j.nextPlaceJobs(), nil
	}

	emails := docEmailExtractor(doc)
	if len(emails) == 0 {
		emails = regexEmailExtractor(resp.Body)
	}

	j.Entry.Emails = emails
	if strings.TrimSpace(j.Entry.Phone) == "" {
		phone := docPhoneExtractor(doc)
		if phone == "" {
			phone = regexPhoneExtractor(resp.Body)
		}

		j.Entry.Phone = phone
	}

	return j.Entry, j.nextPlaceJobs(), nil
}

func (j *EmailExtractJob) ProcessOnFetchError() bool {
	return true
}

func (j *EmailExtractJob) nextPlaceJobs() []scrapemate.IJob {
	cfg := newPlaceJobConfig(j.RootParentID, j.LangCode, j.ExtractEmail, j.ExtractExtraReviews)
	cfg.exitMonitor = j.ExitMonitor
	cfg.writerManaged = j.WriterManagedCompletion

	_, next := newPlaceJobChain(cfg, j.NextPlaces)
	if next == nil {
		return nil
	}

	return []scrapemate.IJob{next}
}

func docEmailExtractor(doc *goquery.Document) []string {
	seen := map[string]bool{}

	var emails []string

	doc.Find("a[href^='mailto:']").Each(func(_ int, s *goquery.Selection) {
		mailto, exists := s.Attr("href")
		if exists {
			value := strings.TrimPrefix(mailto, "mailto:")
			if email, err := getValidEmail(value); err == nil {
				if !seen[email] {
					emails = append(emails, email)
					seen[email] = true
				}
			}
		}
	})

	return emails
}

func regexEmailExtractor(body []byte) []string {
	seen := map[string]bool{}

	var emails []string

	addresses := emailaddress.Find(body, false)
	for i := range addresses {
		if !seen[addresses[i].String()] {
			emails = append(emails, addresses[i].String())
			seen[addresses[i].String()] = true
		}
	}

	return emails
}

func docPhoneExtractor(doc *goquery.Document) string {
	var phone string

	doc.Find("a[href^='tel:']").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		tel, exists := s.Attr("href")
		if !exists {
			return true
		}

		phone = cleanPhone(strings.TrimPrefix(tel, "tel:"))

		return phone == ""
	})

	return phone
}

func regexPhoneExtractor(body []byte) string {
	matches := phoneNumberRe.FindAllString(string(body), -1)
	for _, match := range matches {
		phone := cleanPhone(match)
		if phone != "" {
			return phone
		}
	}

	return ""
}

func cleanPhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.TrimPrefix(value, "tel:")
	value = strings.TrimSpace(strings.Split(value, "?")[0])
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
	compact := replacer.Replace(value)
	digits := 0
	for _, r := range compact {
		if r >= '0' && r <= '9' {
			digits++
			continue
		}
		if r != '+' {
			return ""
		}
	}

	if digits < 8 || digits > 15 {
		return ""
	}

	return value
}

func getValidEmail(s string) (string, error) {
	email, err := emailaddress.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}

	return email.String(), nil
}

// normalizeGoogleURL extracts the actual target URL from Google redirect URLs.
// Google Maps sometimes returns URLs like "/url?q=http://example.com/&opi=..."
// for external website links.
func normalizeGoogleURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	if strings.HasPrefix(rawURL, "/url?q=") {
		fullURL := "https://www.google.com" + rawURL

		parsed, err := url.Parse(fullURL)
		if err != nil {
			return rawURL
		}

		if target := parsed.Query().Get("q"); target != "" {
			return target
		}
	}

	if strings.HasPrefix(rawURL, "/") {
		return "https://www.google.com" + rawURL
	}

	return rawURL
}
