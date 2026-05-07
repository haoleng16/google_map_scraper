package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/web"
)

type desktopStubRunner struct {
	run func(context.Context) error
}

func (r desktopStubRunner) Run(ctx context.Context) error {
	if r.run == nil {
		return nil
	}

	return r.run(ctx)
}

func (r desktopStubRunner) Close(context.Context) error {
	return nil
}

func TestDesktopScraperRunnerCanRestartAfterExit(t *testing.T) {
	service, err := newDesktopScraperService(t.TempDir())
	if err != nil {
		t.Fatalf("create scraper service: %v", err)
	}

	previous := newDesktopWebRunner
	t.Cleanup(func() {
		newDesktopWebRunner = previous
		service.Close(context.Background())
	})

	runCount := 0
	done := make(chan struct{}, 2)
	newDesktopWebRunner = func(*runner.Config) (runner.Runner, error) {
		runCount++
		return desktopStubRunner{run: func(context.Context) error {
			done <- struct{}{}
			return nil
		}}, nil
	}

	if err := service.StartRunner(context.Background(), "cities.db"); err != nil {
		t.Fatalf("start runner first time: %v", err)
	}
	<-done
	waitForRunnerState(t, service, false)

	if err := service.StartRunner(context.Background(), "cities.db"); err != nil {
		t.Fatalf("start runner second time: %v", err)
	}
	<-done
	if runCount != 2 {
		t.Fatalf("runCount = %d, want restarted runner", runCount)
	}
}

func TestScraperListJobsEnsuresRunnerWhenAppStarted(t *testing.T) {
	app := NewApp(t.TempDir())

	previous := newDesktopWebRunner
	t.Cleanup(func() {
		newDesktopWebRunner = previous
		app.shutdown(context.Background())
	})

	started := make(chan struct{}, 1)
	newDesktopWebRunner = func(*runner.Config) (runner.Runner, error) {
		return desktopStubRunner{run: func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done()
			return nil
		}}, nil
	}

	app.ctx = context.Background()
	if _, err := app.ScraperListJobs(); err != nil {
		t.Fatalf("list jobs: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner was not started while listing jobs")
	}
}

func TestScraperCancelJobRestartsRunner(t *testing.T) {
	app := NewApp(t.TempDir())

	previous := newDesktopWebRunner
	t.Cleanup(func() {
		newDesktopWebRunner = previous
		app.shutdown(context.Background())
	})

	started := make(chan int, 2)
	runCount := 0
	newDesktopWebRunner = func(*runner.Config) (runner.Runner, error) {
		runCount++
		n := runCount
		return desktopStubRunner{run: func(ctx context.Context) error {
			started <- n
			<-ctx.Done()
			return nil
		}}, nil
	}

	app.ctx = context.Background()
	created, err := app.ScraperStartJob(ScraperStartRequest{
		Mode:     "country",
		Country:  "VN",
		Keywords: []string{"Cafe"},
	})
	if err != nil {
		t.Fatalf("start scraper job: %v", err)
	}
	<-started

	jobID, _ := created["id"].(string)
	if _, err := app.ScraperCancelJob(jobID); err != nil {
		t.Fatalf("cancel scraper job: %v", err)
	}

	select {
	case n := <-started:
		if n != 2 {
			t.Fatalf("runner generation = %d, want restart generation 2", n)
		}
	case <-time.After(time.Second):
		t.Fatal("runner was not restarted after cancellation")
	}

	job, err := app.scraper.service.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get canceled job: %v", err)
	}
	if job.Status != web.StatusInterrupted {
		t.Fatalf("status = %q, want %q", job.Status, web.StatusInterrupted)
	}
}

func TestStartRunnerRecoversStaleWorkingJobs(t *testing.T) {
	service, err := newDesktopScraperService(t.TempDir())
	if err != nil {
		t.Fatalf("create scraper service: %v", err)
	}

	job := web.Job{
		ID:     "stale-working-job",
		Name:   "stale",
		Date:   time.Now().UTC(),
		Status: web.StatusWorking,
		Data: web.JobData{
			Keywords: []string{"Cafe"},
			Location: "VN",
			Lang:     "en",
			Depth:    desktopScraperDepth,
			MaxTime:  desktopScraperMaxTime,
			Email:    true,
		},
	}
	if err := service.service.Create(context.Background(), &job); err != nil {
		t.Fatalf("create stale job: %v", err)
	}

	previous := newDesktopWebRunner
	t.Cleanup(func() {
		newDesktopWebRunner = previous
		service.Close(context.Background())
	})

	started := make(chan struct{}, 1)
	newDesktopWebRunner = func(*runner.Config) (runner.Runner, error) {
		return desktopStubRunner{run: func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done()
			return nil
		}}, nil
	}

	if err := service.StartRunner(context.Background(), "cities.db"); err != nil {
		t.Fatalf("start runner: %v", err)
	}
	<-started

	recovered, err := service.service.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get recovered job: %v", err)
	}
	if recovered.Status != web.StatusPending {
		t.Fatalf("status = %q, want %q", recovered.Status, web.StatusPending)
	}
}

func waitForRunnerState(t *testing.T, service *DesktopScraperService, active bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if service.RunnerActive() == active {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("runner active = %v, want %v", service.RunnerActive(), active)
}

func TestWhatsAppUploadContactsFileParsesSelectedCSV(t *testing.T) {
	app := NewApp(t.TempDir())
	csvPath := filepath.Join(t.TempDir(), "contacts.csv")
	csvData := []byte("title,phone,category\nCafe One,+1 (555) 123-4567,Cafe\n")
	if err := os.WriteFile(csvPath, csvData, 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	result, err := app.WhatsAppUploadContactsFile(csvPath)
	if err != nil {
		t.Fatalf("upload contacts file: %v", err)
	}

	contacts, ok := result["contacts"].([]map[string]any)
	if !ok {
		t.Fatalf("contacts has type %T, want []map[string]any", result["contacts"])
	}
	if len(contacts) != 1 {
		t.Fatalf("len(contacts) = %d, want 1", len(contacts))
	}
	if contacts[0]["phone"] != "15551234567" {
		t.Fatalf("phone = %q, want cleaned number", contacts[0]["phone"])
	}
	if contacts[0]["selected"] != true {
		t.Fatalf("selected = %v, want true", contacts[0]["selected"])
	}
}

func TestWailsOptionsEnableCSVFileDrop(t *testing.T) {
	app := NewApp(t.TempDir())

	opts := newWailsOptions(app)

	if opts.DragAndDrop == nil {
		t.Fatal("DragAndDrop options are nil")
	}
	if !opts.DragAndDrop.EnableFileDrop {
		t.Fatal("EnableFileDrop = false, want true")
	}
}

func TestNormalizeDesktopExternalURLKeepsGoogleMapsQuery(t *testing.T) {
	raw := "https://www.google.com/maps/place/Test/data=!4m7!3m6?authuser=0&hl=en&rclk=1"
	got, err := normalizeDesktopExternalURL(raw)
	if err != nil {
		t.Fatalf("normalize url: %v", err)
	}
	if got != raw {
		t.Fatalf("url = %q, want original Google Maps URL", got)
	}

	got, err = normalizeDesktopExternalURL("www.google.com/maps/place/Test")
	if err != nil {
		t.Fatalf("normalize url without scheme: %v", err)
	}
	if got != "https://www.google.com/maps/place/Test" {
		t.Fatalf("url = %q", got)
	}
}

func TestOpenExternalURLUsesMacOSOpenCommand(t *testing.T) {
	app := NewApp(t.TempDir())
	raw := "https://www.google.com/maps/place/Test/data=!4m7!3m6?authuser=0&hl=en&rclk=1"

	var opened string
	previous := openDesktopExternalURL
	openDesktopExternalURL = func(url string) error {
		opened = url
		return nil
	}
	t.Cleanup(func() {
		openDesktopExternalURL = previous
	})

	if err := app.OpenExternalURL(raw); err != nil {
		t.Fatalf("open external url: %v", err)
	}
	if opened != raw {
		t.Fatalf("opened = %q, want raw Google Maps URL", opened)
	}
}

func TestScraperStartJobUsesMaximumDesktopDefaults(t *testing.T) {
	app := NewApp(t.TempDir())

	result, err := app.ScraperStartJob(ScraperStartRequest{
		Mode:     "country",
		Country:  "VN",
		Keywords: []string{"restaurant"},
	})
	if err != nil {
		t.Fatalf("start scraper job: %v", err)
	}

	jobID, ok := result["id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("id = %#v, want non-empty string", result["id"])
	}

	job, err := app.scraper.service.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !job.Data.Email {
		t.Fatal("Email = false, want true")
	}
	if job.Data.Depth < 100 {
		t.Fatalf("Depth = %d, want desktop maximum default", job.Data.Depth)
	}
	if job.Data.MaxTime < 7*24*time.Hour {
		t.Fatalf("MaxTime = %s, want at least 7 days", job.Data.MaxTime)
	}
	if job.Data.Location != "VN" {
		t.Fatalf("Location = %q, want VN", job.Data.Location)
	}
}

func TestGeoCountriesUsesChineseDisplayNameWithCountryCode(t *testing.T) {
	app := NewApp(t.TempDir())

	countries, err := app.GeoCountries()
	if err != nil {
		t.Fatalf("load countries: %v", err)
	}

	var china map[string]any
	for _, country := range countries {
		if country["code"] == "CN" {
			china = country
			break
		}
	}
	if china == nil {
		t.Fatal("expected China country row")
	}
	if china["display_name"] != "中国（CN）" {
		t.Fatalf("display_name = %q, want 中国（CN）", china["display_name"])
	}
	if china["value"] != "CN" {
		t.Fatalf("value = %q, want CN", china["value"])
	}
	if count, ok := china["city_count"].(int); !ok || count < 1000 {
		t.Fatalf("city_count = %#v, want broad country coverage", china["city_count"])
	}
}

func TestResultsImportToWhatsAppLoadsPhoneContacts(t *testing.T) {
	app := NewApp(t.TempDir())

	job := testDesktopJob("job-1")
	csvPath := filepath.Join(app.scraper.dataFolder, "job-1.csv")
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/1,Cafe One,Cafe,Main St,{},+1 (555) 123-4567,3,4.5,1,2,hello@example.com,https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := app.results.UpsertCSV(context.Background(), job, csvPath); err != nil {
		t.Fatalf("upsert csv: %v", err)
	}

	result, err := app.ResultsImportToWhatsApp(ResultFilter{HasPhone: true})
	if err != nil {
		t.Fatalf("import results to WhatsApp: %v", err)
	}
	if result["total"] != 1 {
		t.Fatalf("total = %#v, want 1", result["total"])
	}

	contacts, ok := result["contacts"].([]map[string]any)
	if !ok {
		t.Fatalf("contacts has type %T, want []map[string]any", result["contacts"])
	}
	if contacts[0]["phone"] != "15551234567" {
		t.Fatalf("phone = %q, want cleaned phone", contacts[0]["phone"])
	}
}

func TestResultsImportToWhatsAppUsesSelectedJobIDs(t *testing.T) {
	app := NewApp(t.TempDir())

	selectedJob := testDesktopJob("job-selected")
	otherJob := testDesktopJob("job-other")
	if err := os.MkdirAll(app.scraper.dataFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	selectedCSV := filepath.Join(app.scraper.dataFolder, "job-selected.csv")
	otherCSV := filepath.Join(app.scraper.dataFolder, "job-other.csv")
	if err := os.WriteFile(selectedCSV, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/selected,Selected Cafe,Cafe,Main St,{},+1 555 111 0000,3,4.5,1,2,,\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherCSV, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/other,Other Cafe,Cafe,Side St,{},+1 555 222 0000,3,4.5,1,2,,\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), selectedJob, selectedCSV); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), otherJob, otherCSV); err != nil {
		t.Fatal(err)
	}

	result, err := app.ResultsImportToWhatsApp(ResultFilter{JobIDs: []string{"job-selected"}})
	if err != nil {
		t.Fatalf("import selected job results to WhatsApp: %v", err)
	}
	if result["total"] != 1 {
		t.Fatalf("total = %#v, want 1", result["total"])
	}
	contacts, ok := result["contacts"].([]map[string]any)
	if !ok {
		t.Fatalf("contacts has type %T, want []map[string]any", result["contacts"])
	}
	if contacts[0]["shop_name"] != "Selected Cafe" {
		t.Fatalf("shop_name = %q, want selected job contact", contacts[0]["shop_name"])
	}

	otherResults, err := app.results.Search(context.Background(), ResultFilter{JobID: "job-other"})
	if err != nil {
		t.Fatal(err)
	}
	if len(otherResults) != 1 {
		t.Fatalf("other job results = %d, want 1", len(otherResults))
	}
	if otherResults[0].Imported {
		t.Fatal("unselected job result was marked imported")
	}
}

func TestResultStoreKeepsDuplicateBusinessesPerTask(t *testing.T) {
	app := NewApp(t.TempDir())

	firstJob := testDesktopJob("job-first")
	secondJob := testDesktopJob("job-second")
	if err := os.MkdirAll(app.scraper.dataFolder, 0o755); err != nil {
		t.Fatal(err)
	}

	csvData := []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/shared,Shared Cafe,Cafe,Main St,{},+1 555 123 0000,3,4.5,1,2,,\n")
	firstCSV := filepath.Join(app.scraper.dataFolder, "job-first.csv")
	secondCSV := filepath.Join(app.scraper.dataFolder, "job-second.csv")
	if err := os.WriteFile(firstCSV, csvData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondCSV, csvData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := app.results.UpsertCSV(context.Background(), firstJob, firstCSV); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), secondJob, secondCSV); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), firstJob, firstCSV); err != nil {
		t.Fatal(err)
	}

	firstResults, err := app.ResultsSearch(ResultFilter{JobID: firstJob.ID})
	if err != nil {
		t.Fatal(err)
	}
	secondResults, err := app.ResultsSearch(ResultFilter{JobID: secondJob.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstResults) != 1 || len(secondResults) != 1 {
		t.Fatalf("first results = %d, second results = %d, want one result in each task", len(firstResults), len(secondResults))
	}
}

func TestResultStoreFiltersCountryCityAndFormatsRating(t *testing.T) {
	app := NewApp(t.TempDir())

	chinaJob := desktopJob{
		ID:       "job-cn",
		Name:     "中国 cafe",
		Location: "CN",
		Keywords: []string{"Cafe"},
		Date:     time.Now().UTC(),
	}
	tokyoJob := desktopJob{
		ID:       "job-tokyo",
		Name:     "Tokyo dentist",
		Location: "Tokyo, Japan",
		Keywords: []string{"dentist"},
		Date:     time.Now().UTC(),
	}

	chinaCSV := filepath.Join(app.scraper.dataFolder, "job-cn.csv")
	tokyoCSV := filepath.Join(app.scraper.dataFolder, "job-tokyo.csv")
	if err := os.MkdirAll(app.scraper.dataFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chinaCSV, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/cn,Beijing Cafe,Cafe,Beijing China,{},+8613800000000,3,4.567,1,2,,example.cn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokyoCSV, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/tokyo,Tokyo Dental,Dentist,Shinjuku Tokyo,{},+81300000000,4,3,1,2,,example.jp\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := app.results.UpsertCSV(context.Background(), chinaJob, chinaCSV); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), tokyoJob, tokyoCSV); err != nil {
		t.Fatal(err)
	}

	countryResults, err := app.results.Search(context.Background(), ResultFilter{Country: "CN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(countryResults) != 1 || countryResults[0].ShopName != "Beijing Cafe" {
		t.Fatalf("unexpected country results: %+v", countryResults)
	}
	if countryResults[0].Rating != "4.6" {
		t.Fatalf("rating = %q, want 4.6", countryResults[0].Rating)
	}
	if countryResults[0].MapURL != "https://maps.example/cn" {
		t.Fatalf("map url = %q", countryResults[0].MapURL)
	}

	cityResults, err := app.results.Search(context.Background(), ResultFilter{City: "Tokyo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cityResults) != 1 || cityResults[0].ShopName != "Tokyo Dental" {
		t.Fatalf("unexpected city results: %+v", cityResults)
	}
	if cityResults[0].Rating != "3.0" {
		t.Fatalf("rating = %q, want 3.0", cityResults[0].Rating)
	}

	categoryResults, err := app.results.Search(context.Background(), ResultFilter{Category: "Cafe"})
	if err != nil {
		t.Fatal(err)
	}
	if len(categoryResults) != 1 || categoryResults[0].ShopName != "Beijing Cafe" {
		t.Fatalf("unexpected category results: %+v", categoryResults)
	}

	categories, err := app.ResultsCategories(ResultFilter{HasPhone: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(categories, ",") != "Cafe,Dentist" {
		t.Fatalf("categories = %#v, want Cafe,Dentist", categories)
	}
}

func TestResultStoreEnforcesTaskKeywordRelevance(t *testing.T) {
	app := NewApp(t.TempDir())

	job := desktopJob{
		ID:       "job-phone-repair",
		Name:     "CN - 手机维修",
		Location: "CN",
		Keywords: []string{"手机维修"},
		Date:     time.Now().UTC(),
	}
	csvPath := filepath.Join(app.scraper.dataFolder, "job-phone-repair.csv")
	if err := os.MkdirAll(app.scraper.dataFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	csvData := strings.Join([]string{
		"地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网",
		"https://maps.example/phone,极速手机维修,手机维修服务,上海,{},+8613800000001,3,4.8,1,2,,phone.example",
		"https://maps.example/en-phone,Fast Fix,Mobile phone repair shop,Shanghai,{},+8613800000003,3,4.9,1,2,,fastfix.example",
		"https://maps.example/ac,专业空调维修,空调维修服务,上海,{},+8613800000002,3,4.7,1,2,,ac.example",
	}, "\n") + "\n"
	if err := os.WriteFile(csvPath, []byte(csvData), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := app.results.UpsertCSV(context.Background(), job, csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("upsert count = %d, want keyword-matching Chinese and English results", count)
	}

	results, err := app.ResultsSearch(ResultFilter{JobID: "job-phone-repair"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want Chinese and English phone repair only", results)
	}
	resultNames := map[string]bool{}
	for _, result := range results {
		resultNames[result.ShopName] = true
	}
	if !resultNames["极速手机维修"] || !resultNames["Fast Fix"] || resultNames["专业空调维修"] {
		t.Fatalf("results = %+v, want phone repair and no air conditioner repair", results)
	}

	imported, err := app.ResultsImportToWhatsApp(ResultFilter{JobID: "job-phone-repair"})
	if err != nil {
		t.Fatal(err)
	}
	if imported["total"] != 2 {
		t.Fatalf("import total = %#v, want 2", imported["total"])
	}
	contacts, ok := imported["contacts"].([]map[string]any)
	if !ok {
		t.Fatalf("contacts has type %T, want []map[string]any", imported["contacts"])
	}
	importedNames := map[any]bool{}
	for _, contact := range contacts {
		importedNames[contact["shop_name"]] = true
	}
	if !importedNames["极速手机维修"] || !importedNames["Fast Fix"] || importedNames["专业空调维修"] {
		t.Fatalf("imported contacts = %+v, want phone repair and no air conditioner repair", contacts)
	}
}

func TestScraperDeleteJobRemovesJobCSVAndResults(t *testing.T) {
	app := NewApp(t.TempDir())

	started, err := app.ScraperStartJob(ScraperStartRequest{
		Mode:     "country",
		Country:  "CN",
		Keywords: []string{"restaurant"},
	})
	if err != nil {
		t.Fatalf("start scraper job: %v", err)
	}

	jobID, ok := started["id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("id = %#v, want non-empty string", started["id"])
	}

	job := desktopJob{
		ID:       jobID,
		Name:     "CN - restaurant",
		Location: "CN",
		Keywords: []string{"restaurant"},
		Date:     time.Now().UTC(),
		CSVPath:  filepath.Join(app.scraper.dataFolder, jobID+".csv"),
	}
	if err := os.WriteFile(job.CSVPath, []byte("地图网址,商家名称,分类,地址,营业时间,电话,评论数量,评分,纬度,经度,邮箱,官网\nhttps://maps.example/cn,Beijing Cafe,Cafe,Beijing China,{},+8613800000000,3,4.5,1,2,,example.cn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.results.UpsertCSV(context.Background(), job, job.CSVPath); err != nil {
		t.Fatal(err)
	}

	if _, err := app.ScraperDeleteJob(jobID); err != nil {
		t.Fatalf("delete scraper job: %v", err)
	}

	jobs, err := app.ScraperListJobs()
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job["id"] == jobID {
			t.Fatalf("deleted job still listed: %+v", job)
		}
	}

	results, err := app.ResultsSearch(ResultFilter{JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted job results still exist: %+v", results)
	}
	if _, err := os.Stat(job.CSVPath); !os.IsNotExist(err) {
		t.Fatalf("csv still exists or unexpected stat error: %v", err)
	}
}

func testDesktopJob(id string) desktopJob {
	return desktopJob{
		ID:       id,
		Name:     "Vietnam cafe",
		Location: "Vietnam",
		Keywords: []string{"Cafe"},
		Date:     time.Now().UTC(),
	}
}
