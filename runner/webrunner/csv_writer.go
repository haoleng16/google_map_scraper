package webrunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/scrapemate"
)

var _ scrapemate.ResultWriter = (*csvWriter)(nil)

type csvWriter struct {
	out     csvOutput
	filter  *placeFilter
	entries map[string]scrapemate.CsvCapable
	order   []string
	mu      sync.Mutex
}

type csvOutput interface {
	io.Writer
	io.Seeker
	Truncate(size int64) error
}

type placeFilter struct {
	bbox           grid.BoundingBox
	countryAliases map[string]struct{}
}

func newPlaceFilter(bbox grid.BoundingBox, countryCode, countryName string) *placeFilter {
	return &placeFilter{
		bbox:           bbox,
		countryAliases: countryAliases(countryCode, countryName),
	}
}

func newCSVWriter(out csvOutput, filter *placeFilter) scrapemate.ResultWriter {
	return &csvWriter{
		out:     out,
		filter:  filter,
		entries: make(map[string]scrapemate.CsvCapable),
	}
}

func (c *csvWriter) Run(_ context.Context, in <-chan scrapemate.Result) error {
	var filteredCount int

	for result := range in {
		elements, err := csvCapableElements(result.Data)
		if err != nil {
			return err
		}

		if len(elements) == 0 {
			continue
		}

		for _, element := range elements {
			if !c.shouldWrite(element) {
				filteredCount++
				continue
			}

			if err := c.upsert(element); err != nil {
				return err
			}
		}
	}

	log.Printf("csv writer saved %d rows and filtered %d rows", len(c.order), filteredCount)

	return nil
}

func (c *csvWriter) shouldWrite(element scrapemate.CsvCapable) bool {
	if c.filter == nil {
		return true
	}

	entry, ok := element.(*gmaps.Entry)
	if !ok {
		return true
	}

	return c.filter.matches(entry)
}

func (f *placeFilter) matches(entry *gmaps.Entry) bool {
	if !f.bbox.Contains(entry.Latitude, entry.Longtitude) {
		return false
	}

	if len(f.countryAliases) == 0 || strings.TrimSpace(entry.CompleteAddress.Country) == "" {
		return true
	}

	_, ok := f.countryAliases[normalizeCountryName(entry.CompleteAddress.Country)]

	return ok
}

func countryAliases(countryCode, countryName string) map[string]struct{} {
	aliases := make(map[string]struct{})
	addCountryAlias(aliases, countryCode)
	addCountryAlias(aliases, countryName)

	switch strings.ToUpper(strings.TrimSpace(countryCode)) {
	case "CN":
		addCountryAlias(aliases, "China")
		addCountryAlias(aliases, "中国")
	case "GB":
		addCountryAlias(aliases, "United Kingdom")
		addCountryAlias(aliases, "UK")
		addCountryAlias(aliases, "Great Britain")
		addCountryAlias(aliases, "英国")
	case "JP":
		addCountryAlias(aliases, "Japan")
		addCountryAlias(aliases, "日本")
	case "TH":
		addCountryAlias(aliases, "Thailand")
		addCountryAlias(aliases, "Kingdom of Thailand")
		addCountryAlias(aliases, "Thai")
		addCountryAlias(aliases, "泰国")
		addCountryAlias(aliases, "ไทย")
		addCountryAlias(aliases, "ประเทศไทย")
	case "VN":
		addCountryAlias(aliases, "Vietnam")
		addCountryAlias(aliases, "Viet Nam")
		addCountryAlias(aliases, "Việt Nam")
		addCountryAlias(aliases, "越南")
	}

	return aliases
}

func addCountryAlias(aliases map[string]struct{}, value string) {
	value = normalizeCountryName(value)
	if value == "" {
		return
	}

	aliases[value] = struct{}{}
}

func normalizeCountryName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (c *csvWriter) upsert(element scrapemate.CsvCapable) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := csvElementKey(element)
	if key == "" {
		key = fmt.Sprintf("row:%d", len(c.order))
	}

	if existing, ok := c.entries[key]; ok {
		if csvElementCompleteness(existing) >= csvElementCompleteness(element) {
			return nil
		}
	} else {
		c.order = append(c.order, key)
	}

	c.entries[key] = element

	return c.renderLocked()
}

func (c *csvWriter) renderLocked() error {
	if _, err := c.out.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if err := c.out.Truncate(0); err != nil {
		return err
	}

	if len(c.order) == 0 {
		return nil
	}

	w := csv.NewWriter(c.out)
	first := c.entries[c.order[0]]
	if err := w.Write(first.CsvHeaders()); err != nil {
		return err
	}

	for _, key := range c.order {
		if err := w.Write(c.entries[key].CsvRow()); err != nil {
			return err
		}
	}

	w.Flush()

	return w.Error()
}

func csvElementKey(element scrapemate.CsvCapable) string {
	entry, ok := element.(*gmaps.Entry)
	if !ok {
		return ""
	}

	for _, value := range []string{entry.Link, entry.PlaceID, entry.DataID, entry.Cid} {
		value = normalizeEntryKey(value)
		if value != "" {
			return value
		}
	}

	title := strings.ToLower(strings.TrimSpace(entry.Title))
	if title == "" {
		return ""
	}

	return title + "|" + strconv.FormatFloat(entry.Latitude, 'f', 6, 64) + "|" + strconv.FormatFloat(entry.Longtitude, 'f', 6, 64)
}

func normalizeEntryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	if before, _, ok := strings.Cut(value, "?"); ok {
		value = before
	}

	return value
}

func csvElementCompleteness(element scrapemate.CsvCapable) int {
	entry, ok := element.(*gmaps.Entry)
	if !ok {
		return 0
	}

	score := 0
	for _, value := range []string{
		entry.Link,
		entry.Title,
		entry.Category,
		entry.Address,
		entry.Phone,
		entry.WebSite,
		entry.CompleteAddress.Country,
	} {
		if strings.TrimSpace(value) != "" {
			score++
		}
	}

	if len(entry.OpenHours) > 0 {
		score++
	}
	if entry.ReviewCount > 0 {
		score++
	}
	if entry.ReviewRating > 0 {
		score++
	}
	if entry.Latitude != 0 || entry.Longtitude != 0 {
		score++
	}
	if len(entry.Emails) > 0 {
		score++
	}

	return score
}

func csvCapableElements(data any) ([]scrapemate.CsvCapable, error) {
	if data == nil {
		return nil, nil
	}

	if reflect.TypeOf(data).Kind() == reflect.Slice {
		s := reflect.ValueOf(data)

		elements := make([]scrapemate.CsvCapable, 0, s.Len())
		for i := 0; i < s.Len(); i++ {
			val := s.Index(i).Interface()
			element, ok := val.(scrapemate.CsvCapable)
			if !ok {
				return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, val)
			}

			elements = append(elements, element)
		}

		return elements, nil
	}

	element, ok := data.(scrapemate.CsvCapable)
	if !ok {
		return nil, fmt.Errorf("%w: unexpected data type: %T", scrapemate.ErrorNotCsvCapable, data)
	}

	return []scrapemate.CsvCapable{element}, nil
}
