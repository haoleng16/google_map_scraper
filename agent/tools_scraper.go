package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ScraperService abstracts the Google Maps scraper for tool use.
type ScraperService interface {
	// SearchMaps searches for businesses matching the keyword in the given location.
	SearchMaps(ctx context.Context, keyword, location string) (string, error)
}

// toolSearchMaps executes a Google Maps search via the scraper service.
func (a *Agent) toolSearchMaps(ctx context.Context, argsJSON string) string {
	var args struct {
		Keyword  string `json:"keyword"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("解析参数失败: %s", err)
	}

	if args.Keyword == "" || args.Location == "" {
		return "搜索失败：关键词和地点不能为空"
	}

	if a.scraper == nil {
		return fmt.Sprintf("[搜索结果] 关键词: %s, 地点: %s — 爬虫服务未启动", args.Keyword, args.Location)
	}

	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := a.scraper.SearchMaps(searchCtx, args.Keyword, args.Location)
	if err != nil {
		log.Printf("[agent] scraper search error: %v", err)
		return fmt.Sprintf("搜索失败: %s", err.Error())
	}

	return result
}
