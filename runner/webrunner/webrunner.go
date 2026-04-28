package webrunner

import (
	"context"
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
	defaultCityLimit = 120
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

	workerCount := max(1, w.cfg.WebWorkers)
	for i := range workerCount {
		workerID := i + 1
		egroup.Go(func() error {
			return w.work(ctx, workerID)
		})
	}

	egroup.Go(func() error {
		return w.srv.Start(ctx)
	})

	return egroup.Wait()
}

func (w *webrunner) Close(context.Context) error {
	return nil
}

func (w *webrunner) work(ctx context.Context, workerID int) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			job, ok, err := w.svc.ClaimPending(ctx)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			default:
				t0 := time.Now().UTC()
				log.Printf("web worker %d claimed job %s", workerID, job.ID)
				if err := w.scrapeJob(ctx, &job); err != nil {
					params := map[string]any{
						"job_count": len(job.Data.Keywords),
						"duration":  time.Now().UTC().Sub(t0).String(),
						"error":     err.Error(),
					}

					evt := tlmt.NewEvent("web_runner", params)

					_ = runner.Telemetry().Send(ctx, evt)

					log.Printf("error scraping job %s on web worker %d: %v", job.ID, workerID, err)
				} else {
					params := map[string]any{
						"job_count": len(job.Data.Keywords),
						"duration":  time.Now().UTC().Sub(t0).String(),
					}

					_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("web_runner", params))

					log.Printf("job %s scraped successfully on web worker %d", job.ID, workerID)
				}
			}
		}
	}
}

func (w *webrunner) scrapeJob(ctx context.Context, job *web.Job) error {
	if interrupted, err := w.jobInterrupted(ctx, job.ID); err != nil {
		return err
	} else if interrupted {
		return nil
	}

	if len(job.Data.Keywords) == 0 {
		job.Status = web.StatusFailed

		return w.svc.Update(ctx, job)
	}

	jobCtx, jobCancel := context.WithCancel(ctx)
	w.svc.RegisterCancel(job.ID, jobCancel)
	defer w.svc.ClearCancel(job.ID)
	defer jobCancel()

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
	var resultFilter *placeFilter
	if strings.TrimSpace(job.Data.Location) != "" {
		if country, ok, countryErr := localCountryCoverage(jobCtx, job.Data.Location); countryErr != nil {
			log.Printf("job %s could not resolve local country %q: %v", job.ID, job.Data.Location, countryErr)
		} else if ok {
			seedJobs, err = createCountrySeedJobs(job, country, dedup, exitMonitor, w.cfg.ExtraReviews)
			resultFilter = newPlaceFilter(country.BBox, country.CountryCode, country.DisplayName)
		} else if country, ok := geodata.ResolveCountry(job.Data.Location); ok {
			seedJobs, err = createCountrySeedJobs(job, country, dedup, exitMonitor, w.cfg.ExtraReviews)
			resultFilter = newPlaceFilter(country.BBox, country.CountryCode, country.DisplayName)
		} else if resolved, resolveErr := geocode.Resolve(jobCtx, job.Data.Location); resolveErr != nil {
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
			country, countryErr := countryFromCities(jobCtx, resolved)
			if countryErr != nil {
				log.Printf("job %s could not build city coverage for %q (%s): %v; falling back to country bbox grid", job.ID, resolved.DisplayName, resolved.CountryCode, countryErr)
				resultFilter = newPlaceFilter(resolved.BoundingBox, resolved.CountryCode, resolved.DisplayName)
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
				resultFilter = newPlaceFilter(country.BBox, country.CountryCode, country.DisplayName)
			}
		} else {
			resultFilter = newPlaceFilter(resolved.BoundingBox, resolved.CountryCode, resolved.DisplayName)
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
		if interrupted, checkErr := w.jobInterrupted(ctx, job.ID); checkErr == nil && interrupted {
			return nil
		}

		job.Status = web.StatusFailed

		err2 := w.svc.Update(ctx, job)
		if err2 != nil {
			log.Printf("failed to update job status: %v", err2)
		}

		return err
	}

	if interrupted, checkErr := w.jobInterrupted(ctx, job.ID); checkErr != nil {
		return checkErr
	} else if interrupted {
		return nil
	}

	mate, err := w.setupMate(ctx, outfile, job, resultFilter)
	if err != nil {
		if interrupted, checkErr := w.jobInterrupted(ctx, job.ID); checkErr == nil && interrupted {
			return nil
		}

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

		mateCtx, cancel := context.WithTimeout(jobCtx, time.Duration(allowedSeconds)*time.Second)
		defer cancel()

		exitMonitor.SetCancelFunc(cancel)

		go exitMonitor.Run(mateCtx)

		err = mate.Start(mateCtx, seedJobs...)
		if interrupted, checkErr := w.jobInterrupted(ctx, job.ID); checkErr != nil {
			cancel()
			return checkErr
		} else if interrupted {
			cancel()
			return nil
		}

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

	if interrupted, err := w.jobInterrupted(ctx, job.ID); err != nil {
		return err
	} else if interrupted {
		return nil
	}

	job.Status = web.StatusOK

	return w.svc.Update(ctx, job)
}

func (w *webrunner) jobInterrupted(ctx context.Context, id string) (bool, error) {
	job, err := w.svc.Get(ctx, id)
	if err != nil {
		return false, err
	}

	return job.Status == web.StatusInterrupted, nil
}

func (w *webrunner) setupMate(_ context.Context, writer io.Writer, job *web.Job, filter *placeFilter) (*scrapemateapp.ScrapemateApp, error) {
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

	csvOut, ok := writer.(csvOutput)
	if !ok {
		return nil, fmt.Errorf("writer does not support live CSV updates")
	}

	csvWriter := newCSVWriter(csvOut, filter)

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

func localCountryCoverage(ctx context.Context, location string) (geodata.Country, bool, error) {
	store, err := geodata.OpenCityStore(cityDatabasePath)
	if err != nil {
		return geodata.Country{}, false, err
	}
	defer store.Close()

	record, ok, err := store.ResolveCountry(ctx, location)
	if err != nil {
		return geodata.Country{}, false, err
	}

	if !ok {
		return geodata.Country{}, false, nil
	}

	country, err := geodata.CountryFromCityStore(
		ctx,
		store,
		record.Code,
		record.Name,
		record.BBox,
		defaultCityLimit,
	)
	if err != nil {
		return geodata.Country{}, false, err
	}

	if len(country.Areas) == 0 {
		return geodata.Country{}, false, fmt.Errorf("no cities found for country code %s", record.Code)
	}

	return country, true, nil
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
