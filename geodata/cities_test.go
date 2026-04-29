package geodata

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gosom/google-maps-scraper/grid"
	_ "modernc.org/sqlite"
)

func TestCityStoreTopCities(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cities.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
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
		INSERT INTO cities VALUES
			(1, 'Small', 'Small', 'JP', '', 1, 1, 100, 'Asia/Tokyo'),
			(2, 'Tokyo', 'Tokyo', 'JP', '', 35.6762, 139.6503, 9733276, 'Asia/Tokyo'),
			(3, 'London', 'London', 'GB', '', 51.5074, -0.1278, 8961989, 'Europe/London');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenCityStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cities, err := store.TopCities(context.Background(), "JP", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(cities) != 1 || cities[0].Name != "Tokyo" {
		t.Fatalf("unexpected cities: %+v", cities)
	}
}

func TestCountryFromCityStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cities.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
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
		INSERT INTO cities VALUES
			(1, 'Berlin', 'Berlin', 'DE', '', 52.5200, 13.4050, 3677472, 'Europe/Berlin'),
			(2, 'Hamburg', 'Hamburg', 'DE', '', 53.5511, 9.9937, 1906411, 'Europe/Berlin');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenCityStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	country, err := CountryFromCityStore(context.Background(), store, "DE", "Germany", boundingBoxGermany(), 10)
	if err != nil {
		t.Fatal(err)
	}

	if country.CountryCode != "DE" || country.DisplayName != "Germany" {
		t.Fatalf("unexpected country: %+v", country)
	}

	if len(country.Areas) != 2 {
		t.Fatalf("expected 2 areas, got %d", len(country.Areas))
	}
}

func TestCityStoreResolveCountry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cities.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
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
			country_code TEXT NOT NULL
		);
		INSERT INTO countries VALUES ('VN', 'Vietnam', 'Hanoi', 331212, 97338579, 8.0, 102.0, 23.5, 110.0);
		INSERT INTO country_aliases VALUES ('越南', 'VN'), ('vietnam', 'VN');
	`)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenCityStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	country, ok, err := store.ResolveCountry(context.Background(), "越南")
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("expected country to resolve")
	}

	if country.Code != "VN" || country.Name != "Vietnam" {
		t.Fatalf("unexpected country: %+v", country)
	}

	if !country.BBox.Contains(10.8231, 106.6297) {
		t.Fatal("expected Vietnam bbox to contain Ho Chi Minh City")
	}
}

func TestResolveCityDatabasePathReturnsExistingPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cities.db")
	if err := os.WriteFile(dbPath, []byte("sqlite placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveCityDatabasePath(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	if got != dbPath {
		t.Fatalf("expected %q, got %q", dbPath, got)
	}
}

func TestResolveCityDatabasePathMissingIncludesCheckedPaths(t *testing.T) {
	_, err := ResolveCityDatabasePath("missing/cities.db")
	if err == nil {
		t.Fatal("expected missing database error")
	}

	if !strings.Contains(err.Error(), "checked") {
		t.Fatalf("expected checked paths in error, got %q", err.Error())
	}
}

func TestSQLiteReadOnlyDSN(t *testing.T) {
	got, err := sqliteReadOnlyDSN("/tmp/cities.db")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"file:///tmp/cities.db", "immutable=1", "mode=ro"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}

func boundingBoxGermany() grid.BoundingBox {
	return grid.BoundingBox{
		MinLat: 47.270,
		MinLon: 5.866,
		MaxLat: 55.099,
		MaxLon: 15.041,
	}
}
