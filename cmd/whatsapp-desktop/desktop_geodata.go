package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gosom/google-maps-scraper/geodata"
)

//go:embed resources/cities.db
var bundledGeoData embed.FS

const bundledCitiesDBPath = "resources/cities.db"

func ensureDesktopCitiesDB(dataFolder string) (string, error) {
	target := filepath.Join(dataFolder, "geodata", "cities.db")
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		return target, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat desktop city database: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create city database directory: %w", err)
	}

	data, err := bundledGeoData.ReadFile(bundledCitiesDBPath)
	if err != nil {
		return "", fmt.Errorf("read bundled city database: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return "", fmt.Errorf("write desktop city database: %w", err)
	}

	return target, nil
}

func openDesktopCityStore(dataFolder string) (*geodata.CityStore, string, error) {
	path, err := ensureDesktopCitiesDB(dataFolder)
	if err != nil {
		return nil, "", err
	}

	store, err := geodata.OpenCityStore(path)
	if err != nil {
		return nil, "", err
	}

	return store, path, nil
}

func desktopCityStats(ctx context.Context, dataFolder string) (map[string]any, error) {
	store, path, err := openDesktopCityStore(dataFolder)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	stats, err := store.Stats(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"path":          path,
		"country_count": stats.CountryCount,
		"city_count":    stats.CityCount,
	}, nil
}
