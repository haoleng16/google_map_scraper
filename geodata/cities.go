package geodata

import (
	"context"
	"database/sql"
	"fmt"

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
