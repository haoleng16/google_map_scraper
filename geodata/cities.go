package geodata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gosom/google-maps-scraper/grid"
	_ "modernc.org/sqlite"
)

type City struct {
	ID          int64
	Name        string
	ASCIIName   string
	CountryCode string
	Admin1Code  string
	Latitude    float64
	Longitude   float64
	Population  int64
	Timezone    string
}

type CityStore struct {
	db *sql.DB
}

type CountryRecord struct {
	Code       string
	Name       string
	Capital    string
	AreaSqKm   float64
	Population int64
	BBox       grid.BoundingBox
}

func OpenCityStore(path string) (*CityStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	return &CityStore{db: db}, nil
}

func (s *CityStore) Close() error {
	return s.db.Close()
}

func (s *CityStore) TopCities(ctx context.Context, countryCode string, limit int) ([]City, error) {
	if countryCode == "" {
		return nil, fmt.Errorf("country code is empty")
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, ascii_name, country_code, admin1_code,
			latitude, longitude, population, timezone
		FROM cities
		WHERE country_code = ?
		ORDER BY population DESC
		LIMIT ?
	`, countryCode, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cities []City
	for rows.Next() {
		var city City
		if err := rows.Scan(
			&city.ID,
			&city.Name,
			&city.ASCIIName,
			&city.CountryCode,
			&city.Admin1Code,
			&city.Latitude,
			&city.Longitude,
			&city.Population,
			&city.Timezone,
		); err != nil {
			return nil, err
		}

		cities = append(cities, city)
	}

	return cities, rows.Err()
}

func (s *CityStore) ResolveCountry(ctx context.Context, location string) (CountryRecord, bool, error) {
	alias := normalizeCountryAlias(location)
	if alias == "" {
		return CountryRecord{}, false, nil
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT c.country_code, c.name, c.capital, c.area_sq_km, c.population,
			c.min_lat, c.min_lon, c.max_lat, c.max_lon
		FROM country_aliases a
		JOIN countries c ON c.country_code = a.country_code
		WHERE a.alias = ?
	`, alias)

	var country CountryRecord
	err := row.Scan(
		&country.Code,
		&country.Name,
		&country.Capital,
		&country.AreaSqKm,
		&country.Population,
		&country.BBox.MinLat,
		&country.BBox.MinLon,
		&country.BBox.MaxLat,
		&country.BBox.MaxLon,
	)
	if err == nil {
		return country, true, nil
	}
	if err != sql.ErrNoRows {
		return CountryRecord{}, false, err
	}

	return CountryRecord{}, false, nil
}

func normalizeCountryAlias(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
