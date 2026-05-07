package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gosom/google-maps-scraper/geodata"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/runner/installplaywright"
	"github.com/gosom/google-maps-scraper/whatsapp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend
var assets embed.FS

var openDesktopExternalURL = func(url string) error {
	return exec.Command("open", url).Start()
}

func main() {
	dataFolder := defaultDataFolder()
	app := NewApp(dataFolder)

	err := wails.Run(newWailsOptions(app))
	if err != nil {
		log.Fatal(err)
	}
}

func newWailsOptions(app *App) *options.App {
	return &options.App{
		Title:     "WhatsApp 批量发送",
		Width:     1024,
		Height:    768,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: false,
			},
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	}
}

func defaultDataFolder() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".whatsapp-desktop")
}

// App is the Wails binding struct. All public methods are exposed to the frontend.
type App struct {
	ctx        context.Context
	dataFolder string
	service    *whatsapp.Service
	scraper    *DesktopScraperService
	results    *ResultStore
	agentDB    *agentDB
	agentSvc   *AgentService
	initErr    error

	mu          sync.Mutex
	pendingCSV  *map[string]any
	pendingFile *map[string]any
}

func NewApp(dataFolder string) *App {
	scraper, scraperErr := newDesktopScraperService(dataFolder)
	results, resultsErr := newResultStore(filepath.Join(dataFolder, "results.db"))
	adb, agentErr := newAgentDB(dataFolder)
	var initErr error
	if scraperErr != nil {
		initErr = scraperErr
	} else if resultsErr != nil {
		initErr = resultsErr
	} else if agentErr != nil {
		initErr = agentErr
	}

	app := &App{
		dataFolder: dataFolder,
		service:    whatsapp.NewService(dataFolder),
		scraper:    scraper,
		results:    results,
		agentDB:    adb,
		initErr:    initErr,
	}
	if adb != nil {
		app.agentSvc = newAgentService(app, adb)
	}

	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.service.OnProgress = func(event map[string]any) {
		runtime.EventsEmit(ctx, "whatsapp:progress", event)
	}
	if err := a.ensureScraperRunner(ctx); err != nil {
		log.Printf("start desktop scraper runner: %v", err)
	}
}

func (a *App) ensureScraperRunner(ctx context.Context) error {
	if a.initErr != nil || a.scraper == nil {
		return a.initErr
	}
	if ctx == nil {
		return nil
	}

	cityDBPath, err := ensureDesktopCitiesDB(a.dataFolder)
	if err != nil {
		return fmt.Errorf("prepare desktop city database: %w", err)
	}

	return a.scraper.StartRunner(ctx, cityDBPath)
}

func (a *App) restartScraperRunner(ctx context.Context) error {
	if a.initErr != nil || a.scraper == nil {
		return a.initErr
	}
	if ctx == nil {
		return nil
	}

	a.scraper.Close(context.Background())

	return a.ensureScraperRunner(ctx)
}

func (a *App) shutdown(_ context.Context) {
	a.service.Close()
	if a.scraper != nil {
		a.scraper.Close(context.Background())
	}
	if a.results != nil {
		if err := a.results.Close(); err != nil {
			log.Printf("close result store: %v", err)
		}
	}
	if a.agentDB != nil {
		if err := a.agentDB.Close(); err != nil {
			log.Printf("close agent db: %v", err)
		}
	}
}

// === Login ===

// Ping is a simple test method.
func (a *App) Ping() string {
	return "pong"
}

func (a *App) OpenExternalURL(rawURL string) error {
	url, err := normalizeDesktopExternalURL(rawURL)
	if err != nil {
		return err
	}

	return openDesktopExternalURL(url)
}

func (a *App) WhatsAppLoginStart() (map[string]any, error) {
	return a.service.LoginStart()
}

func (a *App) WhatsAppLoginStatus() (map[string]any, error) {
	return a.service.LoginStatus(), nil
}

func (a *App) WhatsAppLogout() error {
	return a.service.Logout()
}

// === Contacts ===

func (a *App) WhatsAppUploadContacts(csvData []byte) (map[string]any, error) {
	result, err := a.service.UploadContacts(csvData)
	if err != nil {
		return nil, err
	}
	return contactsToJSON(result), nil
}

// WhatsAppUploadContactsFile reads a CSV file from disk and imports contacts.
func (a *App) WhatsAppUploadContactsFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return a.WhatsAppUploadContacts(data)
}

func (a *App) WhatsAppListContacts() map[string]any {
	contacts := a.service.ListContacts()
	return map[string]any{
		"total":    len(contacts),
		"contacts": contacts,
	}
}

// === Native File Dialogs ===

// OpenFileDialogForCSV opens a native file picker and stores the result for
// callers that still use the older polling flow.
func (a *App) OpenFileDialogForCSV() {
	a.mu.Lock()
	a.pendingCSV = nil
	a.mu.Unlock()

	go func() {
		path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
			Title: "选择 CSV 联系人文件",
			Filters: []runtime.FileFilter{
				{DisplayName: "CSV 文件", Pattern: "*.csv"},
			},
		})
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": err.Error()}
			a.pendingCSV = &m
			a.mu.Unlock()
			return
		}
		if path == "" {
			return // user cancelled, leave pendingCSV nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": "读取文件失败: " + err.Error()}
			a.pendingCSV = &m
			a.mu.Unlock()
			return
		}
		result, err := a.service.UploadContacts(data)
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": err.Error()}
			a.pendingCSV = &m
			a.mu.Unlock()
			return
		}
		converted := contactsToJSON(result)
		a.mu.Lock()
		a.pendingCSV = &converted
		a.mu.Unlock()
	}()
}

// WhatsAppSelectContactsFile opens the native CSV picker and imports contacts.
func (a *App) WhatsAppSelectContactsFile() (map[string]any, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 CSV 联系人文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "CSV 文件", Pattern: "*.csv"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return a.WhatsAppUploadContactsFile(path)
}

// PollCSVResult returns the pending CSV result if ready, or nil if not yet.
func (a *App) PollCSVResult() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pendingCSV == nil {
		return nil
	}
	result := *a.pendingCSV
	a.pendingCSV = nil
	return result
}

// OpenFileDialogForAttachment opens a native file picker for image or PDF.
func (a *App) OpenFileDialogForAttachment(kind string, msgID int) {
	a.mu.Lock()
	a.pendingFile = nil
	a.mu.Unlock()

	go func() {
		dialogOpts := runtime.OpenDialogOptions{
			Title: "选择图片",
			Filters: []runtime.FileFilter{
				{DisplayName: "图片文件", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp"},
			},
		}
		if kind == "pdf" {
			dialogOpts = runtime.OpenDialogOptions{
				Title: "选择 PDF 文件",
				Filters: []runtime.FileFilter{
					{DisplayName: "PDF 文件", Pattern: "*.pdf"},
				},
			}
		}
		path, err := runtime.OpenFileDialog(a.ctx, dialogOpts)
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": err.Error(), "msg_id": msgID}
			a.pendingFile = &m
			a.mu.Unlock()
			return
		}
		if path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": "读取文件失败: " + err.Error(), "msg_id": msgID}
			a.pendingFile = &m
			a.mu.Unlock()
			return
		}
		name := filepath.Base(path)
		result, err := a.service.UploadFile(data, name)
		if err != nil {
			a.mu.Lock()
			m := map[string]any{"error": err.Error(), "msg_id": msgID}
			a.pendingFile = &m
			a.mu.Unlock()
			return
		}
		a.mu.Lock()
		m := map[string]any{
			"msg_id": msgID,
			"kind":   kind,
			"id":     result["id"],
			"name":   result["name"],
		}
		a.pendingFile = &m
		a.mu.Unlock()
	}()
}

// PollFileResult returns the pending file result if ready, or nil if not yet.
func (a *App) PollFileResult() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pendingFile == nil {
		return nil
	}
	result := *a.pendingFile
	a.pendingFile = nil
	return result
}

// === Send ===

func (a *App) WhatsAppSend(contactIDs []string, messages []whatsapp.Message, options whatsapp.SendOptions) error {
	return a.service.SendStart(contactIDs, messages, options)
}

func (a *App) WhatsAppStop() error {
	return a.service.SendStop()
}

// === Information Retrieval ===

func (a *App) GeoCountries() ([]map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	store, _, err := openDesktopCityStore(a.dataFolder)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	countries, err := store.Countries(context.Background())
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, len(countries))
	for i, country := range countries {
		displayName := country.ChineseName
		if displayName == "" {
			displayName = country.Name
		}
		displayName = displayName + "（" + country.Code + "）"
		out[i] = map[string]any{
			"code":         country.Code,
			"value":        country.Code,
			"name":         country.ChineseName,
			"english_name": country.Name,
			"capital":      country.Capital,
			"population":   country.Population,
			"city_count":   country.CityCount,
			"display_name": displayName,
		}
	}

	return out, nil
}

func (a *App) GeoCities(country string, limit int) ([]map[string]any, error) {
	store, _, err := openDesktopCityStore(a.dataFolder)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	if limit <= 0 {
		limit = 10000
	}
	cities, err := store.TopCities(context.Background(), country, limit)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, len(cities))
	for i, city := range cities {
		out[i] = map[string]any{
			"name":        city.Name,
			"ascii_name":  city.ASCIIName,
			"country":     city.CountryCode,
			"population":  city.Population,
			"latitude":    city.Latitude,
			"longitude":   city.Longitude,
			"displayName": city.ASCIIName + ", " + city.CountryCode,
		}
	}

	return out, nil
}

func (a *App) ScraperStartJob(req ScraperStartRequest) (map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	if err := a.ensureScraperRunner(a.ctx); err != nil {
		return nil, err
	}
	job, err := a.scraper.StartJob(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return scraperJobToJSON(a.scraper.desktopJobFromWebJob(job), 0), nil
}

func (a *App) ScraperListJobs() ([]map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	if err := a.ensureScraperRunner(a.ctx); err != nil {
		return nil, err
	}
	jobs, err := a.scraper.ListJobs(context.Background())
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		count := 0
		if a.results != nil {
			_, _ = a.results.UpsertCSV(context.Background(), job, job.CSVPath)
			total, err := a.results.Count(context.Background(), ResultFilter{JobID: job.ID})
			if err == nil {
				count = total
			}
		}
		out = append(out, scraperJobToJSON(job, count))
	}

	return out, nil
}

func (a *App) ScraperCancelJob(id string) (map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	job, err := a.scraper.CancelJob(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if err := a.restartScraperRunner(a.ctx); err != nil {
		return nil, err
	}

	return scraperJobToJSON(job, 0), nil
}

func (a *App) ScraperDeleteJob(id string) (map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	if err := a.scraper.DeleteJob(context.Background(), id); err != nil {
		return nil, err
	}
	if a.results != nil {
		if err := a.results.DeleteByJobID(context.Background(), id); err != nil {
			return nil, err
		}
	}
	if err := a.restartScraperRunner(a.ctx); err != nil {
		return nil, err
	}

	return map[string]any{"id": id, "deleted": true}, nil
}

func (a *App) ResultsSearch(filter ResultFilter) ([]BusinessResult, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	return a.results.Search(context.Background(), filter)
}

func (a *App) ResultsCategories(filter ResultFilter) ([]string, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	filter.Category = ""
	return a.results.Categories(context.Background(), filter)
}

func (a *App) ResultsImportToWhatsApp(filter ResultFilter) (map[string]any, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	filter.HasPhone = true
	filter.Limit = 50000
	results, err := a.results.Search(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	csvData, ids, err := businessResultsToContactsCSV(results)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]any{"total": 0, "contacts": []map[string]any{}}, nil
	}

	result, err := a.WhatsAppUploadContacts(csvData)
	if err != nil {
		return nil, err
	}
	if err := a.results.MarkImported(context.Background(), ids); err != nil {
		return nil, err
	}

	return result, nil
}

func (a *App) SettingsCheckEnvironment() (map[string]any, error) {
	cityStats, err := desktopCityStats(context.Background(), a.dataFolder)
	if err != nil {
		cityStats = map[string]any{"error": err.Error()}
	}

	return map[string]any{
		"data_folder": a.dataFolder,
		"cities_db":   cityStats,
		"scraper_ok":  a.initErr == nil,
	}, nil
}

func (a *App) SettingsImportCitiesDB(path string) (map[string]any, error) {
	if filepath.Ext(path) != ".db" {
		return nil, os.ErrInvalid
	}
	store, err := geodata.OpenCityStore(path)
	if err != nil {
		return nil, err
	}
	if _, err := store.Stats(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.Close(); err != nil {
		return nil, err
	}

	target := filepath.Join(a.dataFolder, "geodata", "cities.db")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return nil, err
	}
	if a.scraper != nil && a.ctx != nil {
		if err := a.scraper.StartRunner(a.ctx, target); err != nil {
			return nil, err
		}
	}

	return desktopCityStats(context.Background(), a.dataFolder)
}

func (a *App) SettingsSelectCitiesDB() (map[string]any, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 cities.db",
		Filters: []runtime.FileFilter{
			{DisplayName: "SQLite 数据库", Pattern: "*.db"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}

	return a.SettingsImportCitiesDB(path)
}

func (a *App) SettingsInstallBrowsers() error {
	installer, err := installplaywright.New(&runner.Config{RunMode: runner.RunModeInstallPlaywright})
	if err != nil {
		return err
	}

	return installer.Run(context.Background())
}

// === AI Model Config ===

// AgentModelConfig represents the persisted AI model configuration.
type AgentModelConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

func (a *App) agentConfigPath() string {
	return filepath.Join(a.dataFolder, "agent", "model_config.json")
}

// SettingsGetModelConfig returns the saved model configuration.
func (a *App) SettingsGetModelConfig() (AgentModelConfig, error) {
	var cfg AgentModelConfig
	data, err := os.ReadFile(a.agentConfigPath())
	if err != nil {
		// Return defaults if file doesn't exist.
		return AgentModelConfig{
			BaseURL: "https://api.deepseek.com",
			Model:   "deepseek-chat",
		}, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse model config: %w", err)
	}
	return cfg, nil
}

// SettingsSaveModelConfig saves the model configuration to disk.
func (a *App) SettingsSaveModelConfig(cfg AgentModelConfig) error {
	if err := os.MkdirAll(filepath.Dir(a.agentConfigPath()), 0o755); err != nil {
		return fmt.Errorf("create agent config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal model config: %w", err)
	}
	return os.WriteFile(a.agentConfigPath(), data, 0o600)
}

// contactsToJSON converts UploadContacts result to use JSON-style keys.
func contactsToJSON(result map[string]any) map[string]any {
	raw, ok := result["contacts"]
	if !ok {
		return result
	}
	contacts, ok := raw.([]whatsapp.Contact)
	if !ok {
		return result
	}
	out := make([]map[string]any, len(contacts))
	for i, c := range contacts {
		out[i] = map[string]any{
			"id":        c.ID,
			"shop_name": c.ShopName,
			"phone":     c.Phone,
			"address":   c.Address,
			"category":  c.Category,
			"rating":    c.Rating,
			"email":     c.Email,
			"website":   c.Website,
			"selected":  c.Selected,
		}
	}
	return map[string]any{
		"total":    result["total"],
		"contacts": out,
	}
}

func scraperJobToJSON(job desktopJob, importedCount int) map[string]any {
	return map[string]any{
		"id":             job.ID,
		"name":           job.Name,
		"status":         job.Status,
		"location":       job.Location,
		"keywords":       job.Keywords,
		"created_at":     job.Date.Format(time.RFC3339),
		"csv_path":       job.CSVPath,
		"imported_count": importedCount,
	}
}

func normalizeDesktopExternalURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", os.ErrInvalid
	}
	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		return value, nil
	}

	return "https://" + value, nil
}
