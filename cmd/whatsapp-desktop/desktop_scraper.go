package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/runner/webrunner"
	"github.com/gosom/google-maps-scraper/web"
	websqlite "github.com/gosom/google-maps-scraper/web/sqlite"
)

const (
	desktopScraperDepth   = 100
	desktopScraperMaxTime = 7 * 24 * time.Hour
)

type DesktopScraperService struct {
	dataFolder string
	service    *web.Service

	mu         sync.Mutex
	runner     runner.Runner
	cancel     context.CancelFunc
	generation uint64
}

var newDesktopWebRunner = webrunner.New

type ScraperStartRequest struct {
	Mode     string   `json:"mode"`
	Country  string   `json:"country"`
	Location string   `json:"location"`
	Keywords []string `json:"keywords"`
}

type desktopJob struct {
	ID       string
	Name     string
	Status   string
	Location string
	Keywords []string
	Date     time.Time
	CSVPath  string
}

func newDesktopScraperService(dataFolder string) (*DesktopScraperService, error) {
	folder := filepath.Join(dataFolder, "scraper")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, fmt.Errorf("create scraper data folder: %w", err)
	}

	repo, err := websqlite.New(filepath.Join(folder, "jobs.db"))
	if err != nil {
		return nil, fmt.Errorf("open scraper jobs database: %w", err)
	}

	return &DesktopScraperService{
		dataFolder: folder,
		service:    web.NewService(repo, folder),
	}, nil
}

func (s *DesktopScraperService) StartRunner(ctx context.Context, cityDBPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runner != nil {
		return nil
	}
	if err := s.resetStaleWorkingJobs(ctx); err != nil {
		return err
	}

	webrunner.SetCityDatabasePath(cityDBPath)

	runCtx, cancel := context.WithCancel(ctx)
	cfg := &runner.Config{
		DataFolder:       s.dataFolder,
		Addr:             "127.0.0.1:0",
		WebWorkers:       1,
		Concurrency:      1,
		RunMode:          runner.RunModeWeb,
		Email:            true,
		MaxDepth:         desktopScraperDepth,
		Zoom:             15,
		DisablePageReuse: false,
	}

	item, err := newDesktopWebRunner(cfg)
	if err != nil {
		cancel()
		return fmt.Errorf("create scraper runner: %w", err)
	}

	s.runner = item
	s.cancel = cancel
	s.generation++
	generation := s.generation

	go func() {
		if err := item.Run(runCtx); err != nil && runCtx.Err() == nil {
			fmt.Fprintf(os.Stderr, "desktop scraper runner stopped: %v\n", err)
		}

		s.mu.Lock()
		if s.generation == generation {
			s.runner = nil
			s.cancel = nil
		}
		s.mu.Unlock()
	}()

	return nil
}

func (s *DesktopScraperService) Close(ctx context.Context) {
	s.mu.Lock()
	cancel := s.cancel
	item := s.runner
	s.runner = nil
	s.cancel = nil
	s.generation++
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if item != nil {
		_ = item.Close(ctx)
	}
}

func (s *DesktopScraperService) RunnerActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.runner != nil
}

func (s *DesktopScraperService) resetStaleWorkingJobs(ctx context.Context) error {
	jobs, err := s.service.All(ctx)
	if err != nil {
		return fmt.Errorf("load scraper jobs for recovery: %w", err)
	}

	for _, job := range jobs {
		if job.Status != web.StatusWorking {
			continue
		}

		job.Status = web.StatusPending
		if err := s.service.Update(ctx, &job); err != nil {
			return fmt.Errorf("recover scraper job %s: %w", job.ID, err)
		}
	}

	return nil
}

func (s *DesktopScraperService) StartJob(ctx context.Context, req ScraperStartRequest) (web.Job, error) {
	keywords := normalizeKeywords(req.Keywords)
	if len(keywords) == 0 {
		return web.Job{}, fmt.Errorf("请输入关键词")
	}

	location := strings.TrimSpace(req.Location)
	if strings.EqualFold(strings.TrimSpace(req.Mode), "country") || location == "" {
		location = strings.TrimSpace(req.Country)
	}
	if location == "" {
		return web.Job{}, fmt.Errorf("请选择国家或输入城市地区")
	}

	name := location + " - " + strings.Join(keywords, ", ")
	job := web.Job{
		ID:     uuid.NewString(),
		Name:   name,
		Date:   time.Now().UTC(),
		Status: web.StatusPending,
		Data: web.JobData{
			Keywords: keywords,
			Location: location,
			Zoom:     15,
			Depth:    desktopScraperDepth,
			Email:    true,
			MaxTime:  desktopScraperMaxTime,
		},
	}
	job.Data.SetDefaultLang()

	if err := job.Validate(); err != nil {
		return web.Job{}, err
	}
	if err := s.service.Create(ctx, &job); err != nil {
		return web.Job{}, err
	}

	return job, nil
}

func (s *DesktopScraperService) ListJobs(ctx context.Context) ([]desktopJob, error) {
	jobs, err := s.service.All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]desktopJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, s.desktopJobFromWebJob(job))
	}

	return out, nil
}

func (s *DesktopScraperService) CancelJob(ctx context.Context, id string) (desktopJob, error) {
	job, err := s.service.Interrupt(ctx, id)
	if err != nil {
		return desktopJob{}, err
	}

	return s.desktopJobFromWebJob(job), nil
}

func (s *DesktopScraperService) DeleteJob(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("missing job id")
	}

	return s.service.Delete(ctx, id)
}

func (s *DesktopScraperService) desktopJobFromWebJob(job web.Job) desktopJob {
	return desktopJob{
		ID:       job.ID,
		Name:     job.Name,
		Status:   job.Status,
		Location: job.Data.Location,
		Keywords: append([]string(nil), job.Data.Keywords...),
		Date:     job.Date,
		CSVPath:  filepath.Join(s.dataFolder, job.ID+".csv"),
	}
}

func normalizeKeywords(values []string) []string {
	var out []string
	for _, value := range values {
		for _, line := range strings.Split(value, "\n") {
			keyword := strings.TrimSpace(line)
			if keyword == "" {
				continue
			}

			out = append(out, keyword)
		}
	}

	return out
}
