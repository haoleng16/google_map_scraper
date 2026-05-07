package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// scraperAdapter implements agent.ScraperService using DesktopScraperService.
type scraperAdapter struct {
	scraper *DesktopScraperService
}

func (s *scraperAdapter) SearchMaps(ctx context.Context, keyword, location string) (string, error) {
	if s.scraper == nil {
		return "", fmt.Errorf("scraper service not available")
	}

	job, err := s.scraper.StartJob(ctx, ScraperStartRequest{
		Mode:     "city",
		Location: location,
		Keywords: []string{keyword},
	})
	if err != nil {
		return "", fmt.Errorf("start scraper job: %w", err)
	}

	result := s.waitForResults(ctx, job.ID, 25*time.Second)
	if result == "" {
		return fmt.Sprintf("已提交搜索任务（关键词: %s, 地点: %s），爬取中，稍后查看结果。任务ID: %s", keyword, location, job.ID[:8]), nil
	}

	return result, nil
}

func (s *scraperAdapter) waitForResults(ctx context.Context, jobID string, maxWait time.Duration) string {
	deadline := time.Now().Add(maxWait)
	csvPath := s.scraper.dataFolder + "/" + jobID + ".csv"

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(2 * time.Second):
		}

		f, err := os.Open(csvPath)
		if err != nil {
			continue
		}

		results := parseCSVResults(f, 10)
		f.Close()

		if len(results) > 0 {
			return formatResults(results)
		}
	}

	return ""
}

func parseCSVResults(r io.Reader, limit int) []map[string]string {
	reader := csv.NewReader(r)

	header, err := reader.Read()
	if err != nil {
		return nil
	}

	for i, h := range header {
		header[i] = strings.TrimSpace(strings.ToLower(h))
	}

	var results []map[string]string
	for {
		if len(results) >= limit {
			break
		}
		record, err := reader.Read()
		if err != nil {
			break
		}

		row := make(map[string]string)
		for i, val := range record {
			if i < len(header) {
				row[header[i]] = strings.TrimSpace(val)
			}
		}
		results = append(results, row)
	}

	return results
}

func formatResults(results []map[string]string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("找到 %d 条结果：\n\n", len(results)))

	for i, r := range results {
		title := r["title"]
		if title == "" {
			title = r["name"]
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, title))

		if cat := r["category"]; cat != "" {
			b.WriteString(fmt.Sprintf("   分类: %s\n", cat))
		}
		if addr := r["address"]; addr != "" {
			b.WriteString(fmt.Sprintf("   地址: %s\n", addr))
		}
		if phone := r["phone"]; phone != "" {
			b.WriteString(fmt.Sprintf("   电话: %s\n", phone))
		}
		if rating := r["rating"]; rating != "" {
			b.WriteString(fmt.Sprintf("   评分: %s\n", rating))
		}
		if web := r["website"]; web != "" {
			b.WriteString(fmt.Sprintf("   网站: %s\n", web))
		}
		b.WriteString("\n")
	}

	return b.String()
}
