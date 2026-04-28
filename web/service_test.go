package web

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceCountCSVRows(t *testing.T) {
	dataFolder := t.TempDir()
	svc := NewService(nil, dataFolder)

	csvPath := filepath.Join(dataFolder, "job-1.csv")
	err := os.WriteFile(csvPath, []byte("网站,商家名称\nhttps://example.com,Shop 1\nhttps://example.org,Shop 2\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	count, err := svc.CountCSVRows(context.Background(), "job-1")
	if err != nil {
		t.Fatal(err)
	}

	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestServiceCountCSVRowsMissingFile(t *testing.T) {
	svc := NewService(nil, t.TempDir())

	count, err := svc.CountCSVRows(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("expected 0 rows, got %d", count)
	}
}

func TestServiceInterruptMarksJobAndCancels(t *testing.T) {
	ctx := context.Background()
	repo := &memoryJobRepo{
		jobs: map[string]Job{
			"job-1": {
				ID:     "job-1",
				Name:   "job-1",
				Status: StatusWorking,
			},
		},
	}
	svc := NewService(repo, t.TempDir())

	canceled := false
	svc.RegisterCancel("job-1", func() {
		canceled = true
	})

	job, err := svc.Interrupt(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}

	if job.Status != StatusInterrupted {
		t.Fatalf("expected status %q, got %q", StatusInterrupted, job.Status)
	}
	if !canceled {
		t.Fatal("expected running job cancel function to be called")
	}
	if repo.jobs["job-1"].Status != StatusInterrupted {
		t.Fatalf("expected repo status %q, got %q", StatusInterrupted, repo.jobs["job-1"].Status)
	}
}

type memoryJobRepo struct {
	jobs map[string]Job
}

func (r *memoryJobRepo) Get(_ context.Context, id string) (Job, error) {
	return r.jobs[id], nil
}

func (r *memoryJobRepo) Create(_ context.Context, job *Job) error {
	r.jobs[job.ID] = *job
	return nil
}

func (r *memoryJobRepo) Delete(_ context.Context, id string) error {
	delete(r.jobs, id)
	return nil
}

func (r *memoryJobRepo) ClaimPending(context.Context) (Job, bool, error) {
	return Job{}, false, nil
}

func (r *memoryJobRepo) Select(context.Context, SelectParams) ([]Job, error) {
	return nil, nil
}

func (r *memoryJobRepo) Update(_ context.Context, job *Job) error {
	r.jobs[job.ID] = *job
	return nil
}
