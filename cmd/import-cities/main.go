package main

import (
	"bufio"
	"context"
	"database/sql"
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

func main() {
	input := flag.String("input", "", "path to GeoNames cities15000.txt")
	output := flag.String("out", "geodata/cities.db", "path to output SQLite database")
	flag.Parse()

	if *input == "" {
		exitf("-input is required")
	}

	count, err := importCities(context.Background(), *input, *output)
	if err != nil {
		exitf("%v", err)
	}

	fmt.Printf("imported %d cities into %s\n", count, *output)
}

func importCities(ctx context.Context, inputPath, outputPath string) (int, error) {
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
	`)

	return err
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
