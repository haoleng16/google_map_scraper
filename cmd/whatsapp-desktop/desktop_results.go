package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

type ResultStore struct {
	db *sql.DB
}

const maxResultScanLimit = 50000

var resultKeywordSplitter = regexp.MustCompile(`[,\n\r;；、/|]+`)
var resultTokenSplitter = regexp.MustCompile(`[\s,.;:，。；：、/|()（）\[\]{}"'“”‘’+-]+`)

type ResultFilter struct {
	JobID       string   `json:"job_id"`
	JobIDs      []string `json:"job_ids"`
	Query       string   `json:"query"`
	Category    string   `json:"category"`
	Country     string   `json:"country"`
	City        string   `json:"city"`
	Location    string   `json:"location"`
	HasPhone    bool     `json:"has_phone"`
	HasEmail    bool     `json:"has_email"`
	HasWebsite  bool     `json:"has_website"`
	NotImported bool     `json:"not_imported"`
	Limit       int      `json:"limit"`
}

type BusinessResult struct {
	ID          int64  `json:"id"`
	JobID       string `json:"job_id"`
	JobName     string `json:"job_name"`
	Location    string `json:"location"`
	Keywords    string `json:"keywords"`
	MapURL      string `json:"map_url"`
	ShopName    string `json:"shop_name"`
	Category    string `json:"category"`
	Address     string `json:"address"`
	OpenHours   string `json:"open_hours"`
	Phone       string `json:"phone"`
	ReviewCount string `json:"review_count"`
	Rating      string `json:"rating"`
	Latitude    string `json:"latitude"`
	Longitude   string `json:"longitude"`
	Email       string `json:"email"`
	Website     string `json:"website"`
	Imported    bool   `json:"imported"`
	CreatedAt   string `json:"created_at"`
}

func newResultStore(path string) (*ResultStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create result database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := createResultSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &ResultStore{db: db}, nil
}

func (s *ResultStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

func createResultSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS business_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dedupe_key TEXT NOT NULL UNIQUE,
			job_id TEXT NOT NULL,
			job_name TEXT NOT NULL,
			location TEXT NOT NULL,
			keywords TEXT NOT NULL,
			map_url TEXT,
			shop_name TEXT,
			category TEXT,
			address TEXT,
			open_hours TEXT,
			phone TEXT,
			review_count TEXT,
			rating TEXT,
			latitude TEXT,
			longitude TEXT,
			email TEXT,
			website TEXT,
			imported INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_business_results_job_id ON business_results(job_id);
		CREATE INDEX IF NOT EXISTS idx_business_results_phone ON business_results(phone);
		CREATE INDEX IF NOT EXISTS idx_business_results_imported ON business_results(imported);
	`)

	return err
}

func (s *ResultStore) UpsertCSV(ctx context.Context, job desktopJob, csvPath string) (int, error) {
	file, err := os.Open(csvPath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	rows, err := reader.ReadAll()
	if err != nil {
		return 0, err
	}
	if len(rows) <= 1 {
		return 0, nil
	}

	header := csvHeaderIndex(rows[0])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC().Unix()
	count := 0
	if _, err := tx.ExecContext(ctx, `DELETE FROM business_results WHERE job_id = ?`, job.ID); err != nil {
		return 0, err
	}
	for _, row := range rows[1:] {
		item := businessResultFromCSVRow(job, header, row, now)
		if item.dedupeKey == "" || item.ShopName == "" {
			continue
		}
		if !businessResultMatchesKeywords(item.BusinessResult) {
			continue
		}
		if err := upsertBusinessResult(ctx, tx, item); err != nil {
			return 0, err
		}

		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return count, nil
}

func (s *ResultStore) Search(ctx context.Context, filter ResultFilter) ([]BusinessResult, error) {
	query := `
		SELECT id, job_id, job_name, location, keywords, map_url, shop_name,
			category, address, open_hours, phone, review_count, rating, latitude,
			longitude, email, website, imported, created_at
		FROM business_results
		WHERE 1 = 1
	`
	var args []any

	query = appendJobIDFilter(query, filter, &args)
	if strings.TrimSpace(filter.Location) != "" {
		query += " AND location = ?"
		args = append(args, strings.TrimSpace(filter.Location))
	}
	if strings.TrimSpace(filter.Country) != "" {
		query += " AND location = ?"
		args = append(args, strings.TrimSpace(filter.Country))
	}
	if strings.TrimSpace(filter.City) != "" {
		query += " AND (location LIKE ? OR address LIKE ?)"
		pattern := "%" + strings.TrimSpace(filter.City) + "%"
		args = append(args, pattern, pattern)
	}
	if strings.TrimSpace(filter.Query) != "" {
		query += " AND (shop_name LIKE ? OR category LIKE ? OR phone LIKE ? OR email LIKE ? OR website LIKE ? OR address LIKE ?)"
		pattern := "%" + strings.TrimSpace(filter.Query) + "%"
		args = append(args, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	query = appendCategoryFilter(query, filter, &args)
	if filter.HasPhone {
		query += " AND phone <> ''"
	}
	if filter.HasEmail {
		query += " AND email <> ''"
	}
	if filter.HasWebsite {
		query += " AND website <> ''"
	}
	if filter.NotImported {
		query += " AND imported = 0"
	}

	query += " ORDER BY created_at DESC, id DESC"
	limit := normalizedResultLimit(filter.Limit)
	query += " LIMIT ?"
	args = append(args, maxResultScanLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BusinessResult
	for rows.Next() {
		var item BusinessResult
		var imported int
		var createdAt int64
		if err := rows.Scan(
			&item.ID,
			&item.JobID,
			&item.JobName,
			&item.Location,
			&item.Keywords,
			&item.MapURL,
			&item.ShopName,
			&item.Category,
			&item.Address,
			&item.OpenHours,
			&item.Phone,
			&item.ReviewCount,
			&item.Rating,
			&item.Latitude,
			&item.Longitude,
			&item.Email,
			&item.Website,
			&imported,
			&createdAt,
		); err != nil {
			return nil, err
		}
		item.Imported = imported == 1
		item.CreatedAt = time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
		if !businessResultMatchesKeywords(item) {
			continue
		}
		results = append(results, item)
		if len(results) >= limit {
			break
		}
	}

	return results, rows.Err()
}

func (s *ResultStore) Count(ctx context.Context, filter ResultFilter) (int, error) {
	filter.Limit = maxResultScanLimit
	results, err := s.Search(ctx, filter)
	if err != nil {
		return 0, err
	}

	return len(results), nil
}

func (s *ResultStore) Categories(ctx context.Context, filter ResultFilter) ([]string, error) {
	filter.Category = ""
	filter.Limit = maxResultScanLimit
	results, err := s.Search(ctx, filter)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	for _, item := range results {
		category := strings.TrimSpace(item.Category)
		if category == "" {
			continue
		}
		seen[category] = struct{}{}
	}
	categories := make([]string, 0, len(seen))
	for category := range seen {
		categories = append(categories, category)
	}
	sortStringsCaseInsensitive(categories)

	if len(categories) > 500 {
		categories = categories[:500]
	}

	return categories, nil
}

func appendJobIDFilter(query string, filter ResultFilter, args *[]any) string {
	jobIDs := normalizedResultJobIDs(filter)
	if len(jobIDs) == 0 {
		return query
	}
	if len(jobIDs) == 1 {
		*args = append(*args, jobIDs[0])
		return query + " AND job_id = ?"
	}

	placeholders := make([]string, len(jobIDs))
	for i, jobID := range jobIDs {
		placeholders[i] = "?"
		*args = append(*args, jobID)
	}

	return query + " AND job_id IN (" + strings.Join(placeholders, ",") + ")"
}

func appendCategoryFilter(query string, filter ResultFilter, args *[]any) string {
	category := strings.TrimSpace(filter.Category)
	if category == "" {
		return query
	}
	*args = append(*args, category)
	return query + " AND category = ?"
}

func normalizedResultJobIDs(filter ResultFilter) []string {
	seen := make(map[string]struct{}, len(filter.JobIDs)+1)
	jobIDs := make([]string, 0, len(filter.JobIDs)+1)
	appendID := func(jobID string) {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			return
		}
		if _, ok := seen[jobID]; ok {
			return
		}
		seen[jobID] = struct{}{}
		jobIDs = append(jobIDs, jobID)
	}

	appendID(filter.JobID)
	for _, jobID := range filter.JobIDs {
		appendID(jobID)
	}

	return jobIDs
}

func (s *ResultStore) DeleteByJobID(ctx context.Context, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}

	_, err := s.db.ExecContext(ctx, `DELETE FROM business_results WHERE job_id = ?`, jobID)
	return err
}

func (s *ResultStore) MarkImported(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE business_results SET imported = 1, updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type storedBusinessResult struct {
	BusinessResult
	dedupeKey string
	createdAt int64
}

func businessResultFromCSVRow(job desktopJob, header map[string]int, row []string, now int64) storedBusinessResult {
	mapURL := csvCell(row, header, "地图网址", "link", "map_url")
	phone := csvCell(row, header, "电话", "phone")
	shopName := csvCell(row, header, "商家名称", "title", "name")
	address := csvCell(row, header, "地址", "address")

	return storedBusinessResult{
		BusinessResult: BusinessResult{
			JobID:       job.ID,
			JobName:     job.Name,
			Location:    job.Location,
			Keywords:    strings.Join(job.Keywords, ", "),
			MapURL:      mapURL,
			ShopName:    shopName,
			Category:    csvCell(row, header, "分类", "category"),
			Address:     address,
			OpenHours:   csvCell(row, header, "营业时间", "open_hours"),
			Phone:       phone,
			ReviewCount: csvCell(row, header, "评论数量", "review_count"),
			Rating:      formatRatingOneDecimal(csvCell(row, header, "评分", "rating", "review_rating")),
			Latitude:    csvCell(row, header, "纬度", "latitude"),
			Longitude:   csvCell(row, header, "经度", "longitude"),
			Email:       csvCell(row, header, "邮箱", "emails", "email"),
			Website:     csvCell(row, header, "官网", "web_site", "website"),
		},
		dedupeKey: jobScopedBusinessDedupeKey(job.ID, businessDedupeKey(mapURL, phone, shopName, address)),
		createdAt: now,
	}
}

func jobScopedBusinessDedupeKey(jobID, dedupeKey string) string {
	jobID = strings.TrimSpace(jobID)
	dedupeKey = strings.TrimSpace(dedupeKey)
	if jobID == "" || dedupeKey == "" {
		return dedupeKey
	}

	return jobID + "|" + dedupeKey
}

func upsertBusinessResult(ctx context.Context, tx *sql.Tx, item storedBusinessResult) error {
	const query = `
		INSERT INTO business_results (
			dedupe_key, job_id, job_name, location, keywords, map_url, shop_name,
			category, address, open_hours, phone, review_count, rating, latitude,
			longitude, email, website, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dedupe_key) DO UPDATE SET
			job_id = excluded.job_id,
			job_name = excluded.job_name,
			location = excluded.location,
			keywords = excluded.keywords,
			map_url = excluded.map_url,
			shop_name = excluded.shop_name,
			category = excluded.category,
			address = excluded.address,
			open_hours = excluded.open_hours,
			phone = excluded.phone,
			review_count = excluded.review_count,
			rating = excluded.rating,
			latitude = excluded.latitude,
			longitude = excluded.longitude,
			email = excluded.email,
			website = excluded.website,
			updated_at = excluded.updated_at
	`

	_, err := tx.ExecContext(ctx, query,
		item.dedupeKey,
		item.JobID,
		item.JobName,
		item.Location,
		item.Keywords,
		item.MapURL,
		item.ShopName,
		item.Category,
		item.Address,
		item.OpenHours,
		item.Phone,
		item.ReviewCount,
		item.Rating,
		item.Latitude,
		item.Longitude,
		item.Email,
		item.Website,
		item.createdAt,
		time.Now().UTC().Unix(),
	)

	return err
}

func businessDedupeKey(mapURL, phone, shopName, address string) string {
	for _, value := range []string{mapURL, phone + "|" + shopName + "|" + address, shopName + "|" + address} {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && value != "||" && value != "|" {
			return value
		}
	}

	return ""
}

func csvHeaderIndex(headers []string) map[string]int {
	index := make(map[string]int, len(headers))
	for i, header := range headers {
		index[normalizeResultHeader(header)] = i
	}

	return index
}

func csvCell(row []string, header map[string]int, names ...string) string {
	for _, name := range names {
		idx, ok := header[normalizeResultHeader(name)]
		if ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}

	return ""
}

func normalizeResultHeader(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func formatRatingOneDecimal(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	rating, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}

	return fmt.Sprintf("%.1f", rating)
}

func normalizedResultLimit(limit int) int {
	if limit <= 0 {
		return 500
	}
	if limit > maxResultScanLimit {
		return maxResultScanLimit
	}
	return limit
}

func businessResultMatchesKeywords(item BusinessResult) bool {
	keywords := resultKeywords(item.Keywords)
	if len(keywords) == 0 {
		return true
	}

	text := normalizeResultMatchText(strings.Join([]string{
		item.ShopName,
		item.Category,
		item.Address,
		item.Website,
	}, " "))
	if text == "" {
		return false
	}

	for _, keyword := range keywords {
		if resultKeywordMatchesText(keyword, text) {
			return true
		}
	}

	return false
}

func resultKeywords(value string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, part := range resultKeywordSplitter.Split(value, -1) {
		keyword := strings.TrimSpace(part)
		if keyword == "" {
			continue
		}
		normalized := normalizeResultMatchText(keyword)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	return out
}

func resultKeywordMatchesText(keyword, text string) bool {
	if keyword == "" {
		return true
	}
	if strings.Contains(text, keyword) {
		return true
	}

	termGroups := resultKeywordTermGroups(keyword)
	if len(termGroups) == 0 {
		return false
	}
	for _, terms := range termGroups {
		if !resultTextContainsAnyTerm(text, terms) {
			return false
		}
	}

	return true
}

func resultKeywordTermGroups(keyword string) [][]string {
	if containsHan(keyword) {
		return resultTermGroups(chineseKeywordTerms(keyword))
	}

	parts := resultTokenSplitter.Split(keyword, -1)
	groups := make([][]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len([]rune(part)) < 2 || isResultStopWord(part) {
			continue
		}
		groups = append(groups, resultTermAliases(part))
	}
	return groups
}

func chineseKeywordTerms(keyword string) []string {
	runes := []rune(keyword)
	if len(runes) <= 2 {
		return []string{keyword}
	}

	serviceTerms := []string{
		"维修", "修理", "换屏", "回收", "销售", "批发", "租赁", "安装", "清洗",
		"保养", "培训", "诊所", "医院", "牙科", "餐厅", "咖啡", "酒店", "美容",
	}
	for _, service := range serviceTerms {
		if strings.HasSuffix(keyword, service) && len([]rune(keyword)) > len([]rune(service)) {
			prefix := strings.TrimSuffix(keyword, service)
			return []string{prefix, service}
		}
		if idx := strings.Index(keyword, service); idx > 0 {
			prefix := keyword[:idx]
			return []string{prefix, service}
		}
	}

	return []string{keyword}
}

func resultTermGroups(terms []string) [][]string {
	groups := make([][]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		groups = append(groups, resultTermAliases(term))
	}
	return groups
}

func resultTextContainsAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func resultTermAliases(term string) []string {
	term = normalizeResultMatchText(term)
	switch term {
	case "手机":
		return []string{"手机", "phone", "mobile", "mobile phone", "cell phone", "cellphone", "smartphone", "iphone", "android"}
	case "维修", "修理":
		return []string{"维修", "修理", "repair", "repairs", "repairing", "fix", "fixing"}
	case "空调":
		return []string{"空调", "air conditioner", "air conditioning", "hvac", "ac"}
	case "电脑":
		return []string{"电脑", "computer", "pc", "laptop"}
	case "汽车":
		return []string{"汽车", "car", "auto", "automobile", "vehicle"}
	case "餐厅":
		return []string{"餐厅", "restaurant", "restaurants", "dining"}
	case "咖啡":
		return []string{"咖啡", "cafe", "coffee"}
	case "酒店":
		return []string{"酒店", "hotel", "hotels"}
	case "牙科":
		return []string{"牙科", "dentist", "dental"}
	case "美容":
		return []string{"美容", "beauty", "salon", "spa"}
	default:
		return []string{term}
	}
}

func normalizeResultMatchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func isResultStopWord(value string) bool {
	switch value {
	case "a", "an", "and", "for", "in", "near", "of", "the", "to":
		return true
	default:
		return false
	}
}

func sortStringsCaseInsensitive(values []string) {
	sort.Slice(values, func(i, j int) bool {
		return strings.ToLower(values[i]) < strings.ToLower(values[j])
	})
}

func businessResultsToContactsCSV(results []BusinessResult) ([]byte, []int64, error) {
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)
	if err := csvWriter.Write([]string{"商家名称", "电话", "分类", "地址", "评分", "邮箱", "官网"}); err != nil {
		return nil, nil, err
	}

	ids := make([]int64, 0, len(results))
	for _, item := range results {
		if strings.TrimSpace(item.Phone) == "" {
			continue
		}
		if err := csvWriter.Write([]string{
			item.ShopName,
			item.Phone,
			item.Category,
			item.Address,
			item.Rating,
			item.Email,
			item.Website,
		}); err != nil {
			return nil, nil, err
		}
		ids = append(ids, item.ID)
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, nil, err
	}

	return buf.Bytes(), ids, nil
}
