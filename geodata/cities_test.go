package geodata

import (
	"context"
	"database/sql"
	"path/filepath"
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

func boundingBoxGermany() grid.BoundingBox {
	return grid.BoundingBox{
		MinLat: 47.270,
		MinLon: 5.866,
		MaxLat: 55.099,
		MaxLon: 15.041,
	}
}
