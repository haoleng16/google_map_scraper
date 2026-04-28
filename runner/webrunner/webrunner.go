package webrunner

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/geocode"
	"github.com/gosom/google-maps-scraper/geodata"
	"github.com/gosom/google-maps-scraper/grid"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/searchterms"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/google-maps-scraper/web/sqlite"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
	"golang.org/x/sync/errgroup"
)

const (
	cityDatabasePath = "geodata/cities.db"
	defaultCityLimit = 80
)

type webrunner struct {
	srv *web.Server
	svc *web.Service
	cfg *runner.Config
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.DataFolder == "" {
		return nil, fmt.Errorf("data folder is required")
	}

	if err := os.MkdirAll(cfg.DataFolder, os.ModePerm); err != nil {
		return nil, err
	}

	const dbfname = "jobs.db"

	dbpath := filepath.Join(cfg.DataFolder, dbfname)

	repo, err := sqlite.New(dbpath)
	if err != nil {
		return nil, err
	}

	svc := web.NewService(repo, cfg.DataFolder)

	srv, err := web.New(svc, cfg.Addr)
	if err != nil {
		return nil, err
	}

	ans := webrunner{
		srv: srv,
		svc: svc,
		cfg: cfg,
	}

	return &ans, nil
}

func (w *webrunner) Run(ctx context.Context) error {
	egroup, ctx := errgroup.WithContext(ctx)

	egroup.Go(func() error {
		return w.work(ctx)
	})

	egroup.Go(func() error {
		return w.srv.Start(ctx)
	})

	return egroup.Wait()
}

func (w *webrunner) Close(context.Context) error {
	return nil
}

func (w *webrunner) work(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			jobs, err := w.svc.SelectPending(ctx)
			if err != nil {
				return err
			}

			for i := range jobs {
				select {
				case <-ctx.Done():
					return nil
				default:
					t0 := time.Now().UTC()
					if err := w.scrapeJob(ctx, &jobs[i]); err != nil {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
							"error":     err.Error(),
						}

						evt := tlmt.NewEvent("web_runner", params)

						_ = runner.Telemetry().Send(ctx, evt)

						log.Printf("error scraping job %s: %v", jobs[i].ID, err)
					} else {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
						}

						_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("web_runner", params))

						log.Printf("job %s scraped successfully", jobs[i].ID)
					}
				}
			}
		}
	}
}

func (w *webrunner) scrapeJob(ctx context.Context, job *web.Job) error {
	job.Status = web.StatusWorking

	err := w.svc.Update(ctx, job)
	if err != nil {
		return err
	}

	if len(job.Data.Keywords) == 0 {
		job.Status = web.StatusFailed

		return w.svc.Update(ctx, job)
	}

	outpath := filepath.Join(w.cfg.DataFolder, job.ID+".csv")

	outfile, err := os.Create(outpath)
	if err != nil {
		return err
	}

	defer func() {
		_ = outfile.Close()
	}()

	var coords string
	if job.Data.Lat != "" && job.Data.Lon != "" {
		coords = job.Data.Lat + "," + job.Data.Lon
	}

	dedup := deduper.New()
	exitMonitor := exiter.New()
	keywords := searchterms.Expand(job.Data.Keywords)

	var seedJobs []scrapemate.IJob
	var resultFilter *grid.BoundingBox
	if strings.TrimSpace(job.Data.Location) != "" {
		if country, ok := geodata.ResolveCountry(job.Data.Location); ok {
			seedJobs, err = createCountrySeedJobs(job, country, dedup, exitMonitor, w.cfg.ExtraReviews)
			resultFilter = &country.BBox
		} else if resolved, resolveErr := geocode.Resolve(ctx, job.Data.Location); resolveErr != nil {
			log.Printf("job %s could not resolve location %q: %v; falling back to location-qualified search", job.ID, job.Data.Location, resolveErr)
			fallbackKeywords := searchterms.ExpandForLocation(job.Data.Keywords, job.Data.Location)
			langCode := langForLocation(job.Data.Lang, job.Data.Location)
			seedJobs, err = runner.CreateSeedJobs(
				false,
				langCode,
				strings.NewReader(locationQualifiedQueries(fallbackKeywords, job.Data.Location)),
				job.Data.Depth,
				job.Data.Email,
				"",
				job.Data.Zoom,
				10000,
				dedup,
				exitMonitor,
				w.cfg.ExtraReviews || job.Data.ExtraReviews,
			)
		} else if resolved.IsCountry && resolved.CountryCode != "" {
			country, countryErr := countryFromCities(ctx, resolved)
			if countryErr != nil {
				log.Printf("job %s could not build city coverage for %q (%s): %v; falling back to country bbox grid", job.ID, resolved.DisplayName, resolved.CountryCode, countryErr)
				resultFilter = &resolved.BoundingBox
				cellKm := gridCellSizeFor(resolved.BoundingBox)
				gridKeywords := searchterms.ExpandForLocation(job.Data.Keywords, resolved.DisplayName)
				langCode := langForLocation(job.Data.Lang, resolved.DisplayName)

				seedJobs, err = runner.CreateGridSeedJobs(
					langCode,
					strings.NewReader(locationQualifiedQueries(gridKeywords, resolved.DisplayName)),
					job.Data.Depth,
					job.Data.Email,
					resolved.BoundingBox,
					cellKm,
					job.Data.Zoom,
					dedup,
					exitMonitor,
					w.cfg.ExtraReviews || job.Data.ExtraReviews,
				)
			} else {
				seedJobs, err = createCountrySeedJobs(job, country, dedup, exitMonitor, w.cfg.ExtraReviews)
				resultFilter = &country.BBox
			}
		} else {
			resultFilter = &resolved.BoundingBox
			cellKm := gridCellSizeFor(resolved.BoundingBox)
			log.Printf("job %s resolved location %q to %q; grid cell %.1f km", job.ID, job.Data.Location, resolved.DisplayName, cellKm)
			gridKeywords := searchterms.ExpandForLocation(job.Data.Keywords, resolved.DisplayName)
			langCode := langForLocation(job.Data.Lang, resolved.DisplayName)

			seedJobs, err = runner.CreateGridSeedJobs(
				langCode,
				strings.NewReader(locationQualifiedQueries(gridKeywords, resolved.DisplayName)),
				job.Data.Depth,
				job.Data.Email,
				resolved.BoundingBox,
				cellKm,
				job.Data.Zoom,
				dedup,
				exitMonitor,
				w.cfg.ExtraReviews || job.Data.ExtraReviews,
			)
		}
	} else {
		seedJobs, err = runner.CreateSeedJobs(
			job.Data.FastMode,
			job.Data.Lang,
			strings.NewReader(strings.Join(keywords, "\n")),
			job.Data.Depth,
			job.Data.Email,
			coords,
			job.Data.Zoom,
			func() float64 {
				if job.Data.Radius <= 0 {
					return 10000 // 10 km
				}

				return float64(job.Data.Radius)
			}(),
			dedup,
			exitMonitor,
			w.cfg.ExtraReviews || job.Data.ExtraReviews,
		)
	}
	if err != nil {
		job.Status = web.StatusFailed

		err2 := w.svc.Update(ctx, job)
		if err2 != nil {
			log.Printf("failed to update job status: %v", err2)
		}

		return err
	}

	mate, err := w.setupMate(ctx, outfile, job, resultFilter)
	if err != nil {
		job.Status = web.StatusFailed

		err2 := w.svc.Update(ctx, job)
		if err2 != nil {
			log.Printf("failed to update job status: %v", err2)
		}

		return err
	}

	defer mate.Close()

	if len(seedJobs) > 0 {
		exitMonitor.SetSeedCount(len(seedJobs))

		allowedSeconds := max(60, len(seedJobs)*10*job.Data.Depth/50+120)

		if job.Data.MaxTime > 0 {
			if job.Data.MaxTime.Seconds() < 180 {
				allowedSeconds = 180
			} else {
				allowedSeconds = int(job.Data.MaxTime.Seconds())
			}
		}

		log.Printf("running job %s with %d seed jobs and %d allowed seconds", job.ID, len(seedJobs), allowedSeconds)

		mateCtx, cancel := context.WithTimeout(ctx, time.Duration(allowedSeconds)*time.Second)
		defer cancel()

		exitMonitor.SetCancelFunc(cancel)

		go exitMonitor.Run(mateCtx)

		err = mate.Start(mateCtx, seedJobs...)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			cancel()

			err2 := w.svc.Update(ctx, job)
			if err2 != nil {
				log.Printf("failed to update job status: %v", err2)
			}

			return err
		}

		cancel()
	}

	mate.Close()

	job.Status = web.StatusOK

	return w.svc.Update(ctx, job)
}

func (w *webrunner) setupMate(_ context.Context, writer io.Writer, job *web.Job, filter *grid.BoundingBox) (*scrapemateapp.ScrapemateApp, error) {
	opts := []func(*scrapemateapp.Config) error{
		scrapemateapp.WithConcurrency(w.cfg.Concurrency),
		scrapemateapp.WithExitOnInactivity(time.Minute * 3),
	}

	if !job.Data.FastMode {
		opts = append(opts,
			scrapemateapp.WithJS(scrapemateapp.DisableImages()),
		)
	} else {
		opts = append(opts,
			scrapemateapp.WithStealth("firefox"),
		)
	}

	hasProxy := false

	if len(w.cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(w.cfg.Proxies))
		hasProxy = true
	} else if len(job.Data.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(job.Data.Proxies),
		)
		hasProxy = true
	}

	if !w.cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithPageReuseLimit(200),
		)
	}

	log.Printf("job %s has proxy: %v", job.ID, hasProxy)

	csvWriter := newCSVWriter(csv.NewWriter(writer), filter)

	writers := []scrapemate.ResultWriter{csvWriter}

	matecfg, err := scrapemateapp.NewConfig(
		writers,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return scrapemateapp.NewScrapeMateApp(matecfg)
}

func countryFromCities(ctx context.Context, resolved geocode.Result) (geodata.Country, error) {
	store, err := geodata.OpenCityStore(cityDatabasePath)
	if err != nil {
		return geodata.Country{}, err
	}
	defer store.Close()

	country, err := geodata.CountryFromCityStore(
		ctx,
		store,
		resolved.CountryCode,
		resolved.DisplayName,
		resolved.BoundingBox,
		defaultCityLimit,
	)
	if err != nil {
		return geodata.Country{}, err
	}

	if len(country.Areas) == 0 {
		return geodata.Country{}, fmt.Errorf("no cities found for country code %s", resolved.CountryCode)
	}

	return country, nil
}

func createCountrySeedJobs(
	job *web.Job,
	country geodata.Country,
	dedup deduper.Deduper,
	exitMonitor exiter.Exiter,
	extraReviews bool,
) ([]scrapemate.IJob, error) {
	langCode := langForLocation(job.Data.Lang, country.DisplayName)
	countryKeywords := searchterms.ExpandForLocation(job.Data.Keywords, country.DisplayName)
	log.Printf("job %s using maximum coverage plan for %q with %d city areas", job.ID, country.DisplayName, len(country.Areas))

	var seedJobs []scrapemate.IJob
	for _, area := range country.Areas {
		areaJobs, err := runner.CreateGridSeedJobs(
			langCode,
			strings.NewReader(locationQualifiedQueries(countryKeywords, area.Name)),
			job.Data.Depth,
			job.Data.Email,
			area.BBox,
			area.CellKm,
			job.Data.Zoom,
			dedup,
			exitMonitor,
			extraReviews || job.Data.ExtraReviews,
		)
		if err != nil {
			return nil, err
		}

		seedJobs = append(seedJobs, areaJobs...)
	}

	return seedJobs, nil
}

func gridCellSizeFor(bbox grid.BoundingBox) float64 {
	cellKm := 1.0

	for grid.EstimateCellCount(bbox, cellKm) > 500 {
		cellKm *= 2
	}

	return cellKm
}

func locationQualifiedQueries(keywords []string, location string) string {
	var queries []string

	location = strings.TrimSpace(location)
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}

		queries = append(queries, keyword+" "+location)
	}

	return strings.Join(queries, "\n")
}

func langForLocation(currentLang, location string) string {
	if searchterms.IsForeignLocation(location) {
		return "en"
	}

	return currentLang
}
