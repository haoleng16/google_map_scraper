package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Service struct {
	repo          JobRepository
	dataFolder    string
	activeCancels map[string]context.CancelFunc
	activeMu      sync.Mutex
}

func NewService(repo JobRepository, dataFolder string) *Service {
	return &Service{
		repo:          repo,
		dataFolder:    dataFolder,
		activeCancels: make(map[string]context.CancelFunc),
	}
}

func (s *Service) Create(ctx context.Context, job *Job) error {
	return s.repo.Create(ctx, job)
}

func (s *Service) All(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{})
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid file name")
	}

	s.cancelActive(id)

	datapath := filepath.Join(s.dataFolder, id+".csv")

	if _, err := os.Stat(datapath); err == nil {
		if err := os.Remove(datapath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) Update(ctx context.Context, job *Job) error {
	return s.repo.Update(ctx, job)
}

func (s *Service) ClaimPending(ctx context.Context) (Job, bool, error) {
	return s.repo.ClaimPending(ctx)
}

func (s *Service) RegisterCancel(id string, cancel context.CancelFunc) {
	if id == "" || cancel == nil {
		return
	}

	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	s.activeCancels[id] = cancel
}

func (s *Service) ClearCancel(id string) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	delete(s.activeCancels, id)
}

func (s *Service) cancelActive(id string) {
	s.activeMu.Lock()
	cancel := s.activeCancels[id]
	delete(s.activeCancels, id)
	s.activeMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (s *Service) Interrupt(ctx context.Context, id string) (Job, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}

	if job.Status != StatusPending && job.Status != StatusWorking {
		return job, nil
	}

	job.Status = StatusInterrupted
	if err := s.repo.Update(ctx, &job); err != nil {
		return Job{}, err
	}

	s.cancelActive(id)

	return job, nil
}

func (s *Service) SelectPending(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusPending, Limit: 1})
}

func (s *Service) GetCSV(_ context.Context, id string) (string, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file name")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")

	if _, err := os.Stat(datapath); os.IsNotExist(err) {
		return "", fmt.Errorf("csv file not found for job %s", id)
	}

	return datapath, nil
}

func (s *Service) CountCSVRows(_ context.Context, id string) (int, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return 0, fmt.Errorf("invalid file name")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")
	file, err := os.Open(datapath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineCount := 0

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			lineCount++
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			break
		}

		return 0, err
	}

	if lineCount == 0 {
		return 0, nil
	}

	return lineCount - 1, nil
}
