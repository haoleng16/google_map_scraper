package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type cityRecord struct {
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

type countryRecord struct {
	Code       string
	Name       string
	Capital    string
	AreaSqKm   float64
	Population int64
	MinLat     float64
	MinLon     float64
	MaxLat     float64
	MaxLon     float64
}

type countryJSONRecord struct {
	CCA2 string `json:"cca2"`
	Name struct {
		Common   string `json:"common"`
		Official string `json:"official"`
	} `json:"name"`
	Translations map[string]struct {
		Common   string `json:"common"`
		Official string `json:"official"`
	} `json:"translations"`
}

func main() {
	input := flag.String("input", "", "path to GeoNames cities15000.txt")
	countryInfo := flag.String("country-info", "", "path to GeoNames countryInfo.txt")
	countriesJSON := flag.String("countries-json", "", "path to countries.json with translations")
	output := flag.String("out", "geodata/cities.db", "path to output SQLite database")
	flag.Parse()

	if *input == "" {
		exitf("-input is required")
	}

	count, err := importCities(context.Background(), *input, *countryInfo, *countriesJSON, *output)
	if err != nil {
		exitf("%v", err)
	}

	fmt.Printf("imported %d cities into %s\n", count, *output)
}

func importCities(ctx context.Context, inputPath, countryInfoPath, countriesJSONPath, outputPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return 0, err
	}

	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	db, err := sql.Open("sqlite", outputPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	if err := createSchema(ctx, db); err != nil {
		return 0, err
	}

	file, err := os.Open(inputPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO cities (
			id, name, ascii_name, country_code, admin1_code,
			latitude, longitude, population, timezone
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	count := 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		record, err := parseCityRecord(scanner.Text())
		if err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("line %d: %w", lineNumber, err)
		}

		_, err = stmt.ExecContext(ctx,
			record.ID,
			record.Name,
			record.ASCIIName,
			record.CountryCode,
			record.Admin1Code,
			record.Latitude,
			record.Longitude,
			record.Population,
			record.Timezone,
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("line %d: %w", lineNumber, err)
		}

		count++
	}

	if err := scanner.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	if err := createIndexes(ctx, db); err != nil {
		return 0, err
	}

	if countryInfoPath != "" {
		if err := importCountries(ctx, db, countryInfoPath, countriesJSONPath); err != nil {
			return 0, err
		}
	}

	return count, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;

		CREATE TABLE cities (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			ascii_name TEXT NOT NULL,
			country_code TEXT NOT NULL,
			admin1_code TEXT,
			latitude REAL NOT NULL,
			longitude REAL NOT NULL,
			population INTEGER NOT NULL,
			timezone TEXT
		);

		CREATE TABLE countries (
			country_code TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			capital TEXT,
			area_sq_km REAL,
			population INTEGER,
			min_lat REAL NOT NULL,
			min_lon REAL NOT NULL,
			max_lat REAL NOT NULL,
			max_lon REAL NOT NULL
		);

		CREATE TABLE country_aliases (
			alias TEXT PRIMARY KEY,
			country_code TEXT NOT NULL,
			FOREIGN KEY(country_code) REFERENCES countries(country_code)
		);
	`)

	return err
}

func createIndexes(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE INDEX idx_cities_country_population
		ON cities(country_code, population DESC);

		CREATE INDEX idx_cities_name
		ON cities(name);

		CREATE INDEX idx_cities_ascii_name
		ON cities(ascii_name);

		CREATE INDEX idx_country_aliases_country_code
		ON country_aliases(country_code);
	`)

	return err
}

func importCountries(ctx context.Context, db *sql.DB, countryInfoPath, countriesJSONPath string) error {
	records, err := readCountryInfo(countryInfoPath)
	if err != nil {
		return err
	}

	translationAliases, err := readCountryTranslationAliases(countriesJSONPath)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	countryStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO countries (
			country_code, name, capital, area_sq_km, population,
			min_lat, min_lon, max_lat, max_lon
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer countryStmt.Close()

	aliasStmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO country_aliases (alias, country_code)
		VALUES (?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer aliasStmt.Close()

	for _, record := range records {
		bbox, ok, err := countryBoundingBox(ctx, tx, record.Code)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if !ok {
			continue
		}

		record.MinLat = bbox.minLat
		record.MinLon = bbox.minLon
		record.MaxLat = bbox.maxLat
		record.MaxLon = bbox.maxLon

		_, err = countryStmt.ExecContext(ctx,
			record.Code,
			record.Name,
			record.Capital,
			record.AreaSqKm,
			record.Population,
			record.MinLat,
			record.MinLon,
			record.MaxLat,
			record.MaxLon,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		aliases := countryAliases(record, translationAliases[record.Code])
		for _, alias := range aliases {
			_, err = aliasStmt.ExecContext(ctx, normalizeAlias(alias), record.Code)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}

	return tx.Commit()
}

func readCountryInfo(path string) ([]countryRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []countryRecord
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		record, err := parseCountryRecord(line)
		if err != nil {
			return nil, fmt.Errorf("country info line %d: %w", lineNumber, err)
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func parseCountryRecord(line string) (countryRecord, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 8 {
		return countryRecord{}, fmt.Errorf("expected at least 8 fields, got %d", len(fields))
	}

	area, err := parseOptionalFloat(fields[6])
	if err != nil {
		return countryRecord{}, fmt.Errorf("invalid area: %w", err)
	}

	population, err := parseOptionalInt(fields[7])
	if err != nil {
		return countryRecord{}, fmt.Errorf("invalid population: %w", err)
	}

	return countryRecord{
		Code:       strings.ToUpper(strings.TrimSpace(fields[0])),
		Name:       strings.TrimSpace(fields[4]),
		Capital:    strings.TrimSpace(fields[5]),
		AreaSqKm:   area,
		Population: population,
	}, nil
}

func readCountryTranslationAliases(path string) (map[string][]string, error) {
	aliases := make(map[string][]string)
	if path == "" {
		return aliases, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var records []countryJSONRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}

	for _, record := range records {
		code := strings.ToUpper(strings.TrimSpace(record.CCA2))
		if code == "" {
			continue
		}

		addAlias(&aliases, code, record.Name.Common)
		addAlias(&aliases, code, record.Name.Official)
		if zho, ok := record.Translations["zho"]; ok {
			addAlias(&aliases, code, zho.Common)
			addAlias(&aliases, code, zho.Official)
		}
	}

	return aliases, nil
}

type bboxRecord struct {
	minLat float64
	minLon float64
	maxLat float64
	maxLon float64
}

func countryBoundingBox(ctx context.Context, tx *sql.Tx, countryCode string) (bboxRecord, bool, error) {
	var minLat sql.NullFloat64
	var minLon sql.NullFloat64
	var maxLat sql.NullFloat64
	var maxLon sql.NullFloat64
	err := tx.QueryRowContext(ctx, `
		SELECT MIN(latitude), MIN(longitude), MAX(latitude), MAX(longitude)
		FROM cities
		WHERE country_code = ?
	`, countryCode).Scan(&minLat, &minLon, &maxLat, &maxLon)
	if err != nil {
		return bboxRecord{}, false, err
	}

	if !minLat.Valid || !minLon.Valid || !maxLat.Valid || !maxLon.Valid {
		return bboxRecord{}, false, nil
	}

	bbox := bboxRecord{
		minLat: minLat.Float64,
		minLon: minLon.Float64,
		maxLat: maxLat.Float64,
		maxLon: maxLon.Float64,
	}

	const padding = 0.75
	bbox.minLat = max(-90, bbox.minLat-padding)
	bbox.minLon = max(-180, bbox.minLon-padding)
	bbox.maxLat = min(90, bbox.maxLat+padding)
	bbox.maxLon = min(180, bbox.maxLon+padding)

	return bbox, true, nil
}

func countryAliases(record countryRecord, translated []string) []string {
	var aliases []string
	seen := make(map[string]struct{})

	addUniqueAlias(&aliases, seen, record.Code)
	addUniqueAlias(&aliases, seen, record.Name)
	addUniqueAlias(&aliases, seen, strings.ReplaceAll(record.Name, ",", ""))

	for _, alias := range translated {
		addUniqueAlias(&aliases, seen, alias)
	}

	for _, alias := range manualCountryAliases()[record.Code] {
		addUniqueAlias(&aliases, seen, alias)
	}

	return aliases
}

func manualCountryAliases() map[string][]string {
	return map[string][]string{
		"AE": {"阿联酋", "United Arab Emirates", "UAE"},
		"BR": {"巴西", "Brazil"},
		"CN": {"中国", "中华人民共和国", "China"},
		"DE": {"德国", "Germany"},
		"ES": {"西班牙", "Spain"},
		"FR": {"法国", "France"},
		"GB": {"英国", "UK", "United Kingdom", "Great Britain"},
		"IN": {"印度", "India"},
		"IT": {"意大利", "Italy"},
		"JP": {"日本", "Japan"},
		"KR": {"韩国", "南韩", "South Korea", "Korea"},
		"MX": {"墨西哥", "Mexico"},
		"MY": {"马来西亚", "Malaysia"},
		"RU": {"俄罗斯", "Russia"},
		"SG": {"新加坡", "Singapore"},
		"TH": {"泰国", "Thailand", "ประเทศไทย"},
		"US": {"美国", "United States", "USA", "America"},
		"VN": {"越南", "Vietnam", "Viet Nam", "Việt Nam"},
	}
}

func addAlias(aliases *map[string][]string, code, alias string) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}

	(*aliases)[code] = append((*aliases)[code], alias)
}

func addUniqueAlias(aliases *[]string, seen map[string]struct{}, alias string) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}

	key := normalizeAlias(alias)
	if _, ok := seen[key]; ok {
		return
	}

	seen[key] = struct{}{}
	*aliases = append(*aliases, alias)
}

func normalizeAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func parseOptionalFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	return strconv.ParseFloat(value, 64)
}

func parseOptionalInt(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	return strconv.ParseInt(value, 10, 64)
}

func parseCityRecord(line string) (cityRecord, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < 19 {
		return cityRecord{}, fmt.Errorf("expected at least 19 fields, got %d", len(fields))
	}

	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return cityRecord{}, fmt.Errorf("invalid geoname id: %w", err)
	}

	lat, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return cityRecord{}, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return cityRecord{}, fmt.Errorf("invalid longitude: %w", err)
	}

	population, err := strconv.ParseInt(fields[14], 10, 64)
	if err != nil {
		return cityRecord{}, fmt.Errorf("invalid population: %w", err)
	}

	return cityRecord{
		ID:          id,
		Name:        fields[1],
		ASCIIName:   fields[2],
		CountryCode: fields[8],
		Admin1Code:  fields[10],
		Latitude:    lat,
		Longitude:   lon,
		Population:  population,
		Timezone:    fields[17],
	}, nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
