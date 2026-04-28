package webrunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"reflect"
	"sync"

	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/scrapemate"
)

var _ scrapemate.ResultWriter = (*csvWriter)(nil)

type csvWriter struct {
	w      *csv.Writer
	filter *grid.BoundingBox
	once   sync.Once
}

func newCSVWriter(w *csv.Writer, filter *grid.BoundingBox) scrapemate.ResultWriter {
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

	return c.filter.Contains(entry.Latitude, entry.Longtitude)
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
