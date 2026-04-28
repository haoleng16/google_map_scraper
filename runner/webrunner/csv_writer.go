package webrunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/scrapemate"
)

var _ scrapemate.ResultWriter = (*csvWriter)(nil)

type csvWriter struct {
	w      *csv.Writer
	filter *placeFilter
	once   sync.Once
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

func newCSVWriter(w *csv.Writer, filter *placeFilter) scrapemate.ResultWriter {
	return &csvWriter{
		w:      w,
		filter: filter,
	}
}

func (c *csvWriter) Run(_ context.Context, in <-chan scrapemate.Result) error {
	for result := range in {
		elements, err := csvCapableElements(result.Data)
		if err != nil {
			return err
		}

		if len(elements) == 0 {
			continue
		}

		c.once.Do(func() {
			_ = c.w.Write(elements[0].CsvHeaders())
		})

		for _, element := range elements {
			if !c.shouldWrite(element) {
				continue
			}

			if err := c.w.Write(element.CsvRow()); err != nil {
				return err
			}
		}

		c.w.Flush()
	}

	return c.w.Error()
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
		addCountryAlias(aliases, "Thai")
		addCountryAlias(aliases, "泰国")
		addCountryAlias(aliases, "ไทย")
		addCountryAlias(aliases, "ประเทศไทย")
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
