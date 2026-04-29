package geodata

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	resolvedPath, err := ResolveCityDatabasePath(path)
	if err != nil {
		return nil, err
	}

	dsn, err := sqliteReadOnlyDSN(resolvedPath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open city database %q: %w", resolvedPath, err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open city database %q: %w", resolvedPath, err)
	}

	return &CityStore{db: db}, nil
}

func ResolveCityDatabasePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("city database path is empty")
	}

	candidates := cityDatabasePathCandidates(path)
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("city database path %q is a directory", candidate)
			}

			return candidate, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat city database %q: %w", candidate, err)
		}
	}

	return "", fmt.Errorf("city database %q not found; checked %s", path, strings.Join(candidates, ", "))
}

func cityDatabasePathCandidates(path string) []string {
	if filepath.IsAbs(path) {
		return []string{path}
	}

	candidates := []string{path}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, path))
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), path))
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(exe)), path))
	}

	return uniquePaths(candidates)
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}

		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	return out
}

func sqliteReadOnlyDSN(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve city database absolute path %q: %w", path, err)
	}

	uri := url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	uri.RawQuery = query.Encode()

	return uri.String(), nil
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
