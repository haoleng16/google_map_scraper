package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/grid"
)

const nominatimSearchURL = "https://nominatim.openstreetmap.org/search"
const requestTimeout = 30 * time.Second
const maxAttempts = 3

type Result struct {
	BoundingBox grid.BoundingBox
	DisplayName string
	CountryCode string
	IsCountry   bool
}

type searchResult struct {
	BoundingBox []string `json:"boundingbox"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Addresstype string   `json:"addresstype"`
	Address     struct {
		CountryCode string `json:"country_code"`
	} `json:"address"`
}

func Resolve(ctx context.Context, location string) (Result, error) {
	if result, ok := resolveKnownLocation(location); ok {
		return result, nil
	}

	return resolve(ctx, http.DefaultClient, nominatimSearchURL, location)
}

func resolve(ctx context.Context, client *http.Client, endpoint, location string) (Result, error) {
	if location == "" {
		return Result{}, fmt.Errorf("location is empty")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return Result{}, err
	}

	q := u.Query()
	q.Set("q", location)
	q.Set("format", "jsonv2")
	q.Set("limit", "1")
	q.Set("addressdetails", "1")
	u.RawQuery = q.Encode()

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
		if err != nil {
			cancel()
			return Result{}, err
		}

		req.Header.Set("User-Agent", "google-maps-scraper/1.0")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		result, err := decodeSearchResponse(resp, location)
		cancel()
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return Result{}, lastErr
}

func decodeSearchResponse(resp *http.Response, location string) (Result, error) {
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, fmt.Errorf("geocode request failed with status %d", resp.StatusCode)
	}

	var results []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return Result{}, err
	}

	if len(results) == 0 {
		return Result{}, fmt.Errorf("location %q not found", location)
	}

	bbox, err := parseBoundingBox(results[0].BoundingBox)
	if err != nil {
		return Result{}, err
	}

	return Result{
		BoundingBox: bbox,
		DisplayName: results[0].DisplayName,
		CountryCode: strings.ToUpper(results[0].Address.CountryCode),
		IsCountry:   results[0].Addresstype == "country" || results[0].Type == "country",
	}, nil
}

func resolveKnownLocation(location string) (Result, bool) {
	key := strings.ToLower(strings.TrimSpace(location))
	key = strings.TrimSuffix(key, "市")

	known := map[string]struct {
		displayName string
		countryCode string
		isCountry   bool
		bbox        grid.BoundingBox
	}{
		"深圳": {
			displayName: "深圳市, 广东省, 中国",
			countryCode: "CN",
			bbox: grid.BoundingBox{
				MinLat: 22.3964,
				MinLon: 113.7520,
				MaxLat: 22.8617,
				MaxLon: 114.6285,
			},
		},
		"shenzhen": {
			displayName: "Shenzhen, Guangdong, China",
			countryCode: "CN",
			bbox: grid.BoundingBox{
				MinLat: 22.3964,
				MinLon: 113.7520,
				MaxLat: 22.8617,
				MaxLon: 114.6285,
			},
		},
		"英国": {
			displayName: "United Kingdom",
			countryCode: "GB",
			isCountry:   true,
			bbox: grid.BoundingBox{
				MinLat: 49.6740,
				MinLon: -8.6500,
				MaxLat: 61.0610,
				MaxLon: 1.7680,
			},
		},
		"united kingdom": {
			displayName: "United Kingdom",
			countryCode: "GB",
			isCountry:   true,
			bbox: grid.BoundingBox{
				MinLat: 49.6740,
				MinLon: -8.6500,
				MaxLat: 61.0610,
				MaxLon: 1.7680,
			},
		},
		"uk": {
			displayName: "United Kingdom",
			countryCode: "GB",
			isCountry:   true,
			bbox: grid.BoundingBox{
				MinLat: 49.6740,
				MinLon: -8.6500,
				MaxLat: 61.0610,
				MaxLon: 1.7680,
			},
		},
		"日本": {
			displayName: "Japan",
			countryCode: "JP",
			isCountry:   true,
			bbox: grid.BoundingBox{
				MinLat: 24.0,
				MinLon: 122.0,
				MaxLat: 46.0,
				MaxLon: 146.0,
			},
		},
		"japan": {
			displayName: "Japan",
			countryCode: "JP",
			isCountry:   true,
			bbox: grid.BoundingBox{
				MinLat: 24.0,
				MinLon: 122.0,
				MaxLat: 46.0,
				MaxLon: 146.0,
			},
		},
	}

	result, ok := known[key]
	if !ok {
		return Result{}, false
	}

	return Result{
		BoundingBox: result.bbox,
		DisplayName: result.displayName,
		CountryCode: result.countryCode,
		IsCountry:   result.isCountry,
	}, true
}

func parseBoundingBox(raw []string) (grid.BoundingBox, error) {
	if len(raw) != 4 {
		return grid.BoundingBox{}, fmt.Errorf("invalid geocode bounding box")
	}

	minLat, err := strconv.ParseFloat(raw[0], 64)
	if err != nil {
		return grid.BoundingBox{}, fmt.Errorf("invalid min latitude: %w", err)
	}

	maxLat, err := strconv.ParseFloat(raw[1], 64)
	if err != nil {
		return grid.BoundingBox{}, fmt.Errorf("invalid max latitude: %w", err)
	}

	minLon, err := strconv.ParseFloat(raw[2], 64)
	if err != nil {
		return grid.BoundingBox{}, fmt.Errorf("invalid min longitude: %w", err)
	}

	maxLon, err := strconv.ParseFloat(raw[3], 64)
	if err != nil {
		return grid.BoundingBox{}, fmt.Errorf("invalid max longitude: %w", err)
	}

	return grid.ParseBoundingBox(fmt.Sprintf("%f,%f,%f,%f", minLat, minLon, maxLat, maxLon))
}
