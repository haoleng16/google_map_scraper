package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

func TestClaimPendingMarksOneJobWorking(t *testing.T) {
	repo, err := New(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	job := testJob("job-1")
	if err := repo.Create(ctx, &job); err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := repo.ClaimPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a pending job to be claimed")
	}
	if claimed.ID != job.ID {
		t.Fatalf("expected job %q, got %q", job.ID, claimed.ID)
	}
	if claimed.Status != web.StatusWorking {
		t.Fatalf("expected status %q, got %q", web.StatusWorking, claimed.Status)
	}

	_, ok, err = repo.ClaimPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no pending jobs after claim")
	}
}

func TestClaimPendingConcurrentClaimsAreUnique(t *testing.T) {
	repo, err := New(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	const jobCount = 5
	for i := range jobCount {
		job := testJob(fmt.Sprintf("job-%d", i))
		if err := repo.Create(ctx, &job); err != nil {
			t.Fatal(err)
		}
	}

	const claimers = 10
	var wg sync.WaitGroup
	claimedIDs := make(chan string, claimers)
	errs := make(chan error, claimers)
	for range claimers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, ok, err := repo.ClaimPending(ctx)
			if err != nil {
				errs <- err
				return
			}
			if ok {
				claimedIDs <- claimed.ID
			}
		}()
	}

	wg.Wait()
	close(claimedIDs)
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	for id := range claimedIDs {
		if seen[id] {
			t.Fatalf("job %q was claimed more than once", id)
		}
		seen[id] = true
	}

	if len(seen) != jobCount {
		t.Fatalf("expected %d claimed jobs, got %d", jobCount, len(seen))
	}
}

func testJob(id string) web.Job {
	return web.Job{
		ID:     id,
		Name:   id,
		Status: web.StatusPending,
		Date:   time.Now().UTC(),
		Data: web.JobData{
			Keywords: []string{"mobile phone shop"},
			Lang:     "en",
			Zoom:     15,
			Depth:    1,
			Radius:   10000,
			MaxTime:  time.Minute,
		},
	}
}
