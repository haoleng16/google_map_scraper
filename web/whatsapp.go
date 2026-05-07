package web

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/playwright-community/playwright-go"
)

const (
	maxWhatsAppImageSize = 10 * 1024 * 1024
	maxWhatsAppPDFSize   = 20 * 1024 * 1024
)

var phoneCleaner = regexp.MustCompile(`[+\-\s()]`)

type whatsAppService struct {
	sender     *whatsAppSender
	contacts   []whatsAppContact
	files      map[string]whatsAppFile
	uploadsDir string

	mu        sync.Mutex
	sendLock  sync.Mutex
	stop      chan struct{}
	sending   bool
	listeners map[chan string]struct{}
}

type whatsAppContact struct {
	ID       string `json:"id"`
	ShopName string `json:"shop_name"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	Category string `json:"category"`
	Rating   string `json:"rating"`
	Email    string `json:"email"`
	Website  string `json:"website"`
	Selected bool   `json:"selected"`
}

type whatsAppFile struct {
	Path string `json:"-"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type whatsAppMessage struct {
	Text      string `json:"text"`
	ImageID   string `json:"image_id"`
	PDFID     string `json:"pdf_id"`
	ImagePath string `json:"-"`
	PDFPath   string `json:"-"`
}

func newWhatsAppService(dataFolder string) *whatsAppService {
	base := filepath.Join(dataFolder, "whatsapp")
	uploads := filepath.Join(base, "uploads")

	return &whatsAppService{
		sender:     newWhatsAppSender(filepath.Join(base, "session"), filepath.Join(base, "debug_screenshots")),
		files:      make(map[string]whatsAppFile),
		uploadsDir: uploads,
		stop:       make(chan struct{}),
		listeners:  make(map[chan string]struct{}),
	}
}

func (s *whatsAppService) close() {
	if s == nil {
		return
	}
	if err := s.sender.close(); err != nil {
		log.Printf("close WhatsApp sender: %v", err)
	}
}

func (s *Server) whatsAppAPI(w http.ResponseWriter, r *http.Request) {
	if s.whatsApp == nil {
		renderWhatsAppError(w, http.StatusServiceUnavailable, "WhatsApp 服务未初始化", "ERR_UNAVAILABLE")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/whatsapp/api")
	switch {
	case path == "/login/start" && r.Method == http.MethodPost:
		s.whatsApp.loginStart(w, r)
	case path == "/login/status" && r.Method == http.MethodGet:
		s.whatsApp.loginStatus(w, r)
	case path == "/login/logout" && r.Method == http.MethodPost:
		s.whatsApp.loginLogout(w, r)
	case path == "/contacts/upload" && r.Method == http.MethodPost:
		s.whatsApp.contactsUpload(w, r)
	case path == "/contacts" && r.Method == http.MethodGet:
		s.whatsApp.contactsList(w, r)
	case path == "/upload" && r.Method == http.MethodPost:
		s.whatsApp.uploadFile(w, r)
	case path == "/send" && r.Method == http.MethodPost:
		s.whatsApp.sendStart(w, r)
	case path == "/send/stop" && r.Method == http.MethodPost:
		s.whatsApp.sendStop(w, r)
	case path == "/send/status" && r.Method == http.MethodGet:
		s.whatsApp.sendStatus(w, r)
	case path == "/send/events" && r.Method == http.MethodGet:
		s.whatsApp.sendEvents(w, r)
	default:
		renderWhatsAppError(w, http.StatusNotFound, "未找到 WhatsApp API", "ERR_NOT_FOUND")
	}
}

func (s *whatsAppService) loginStart(w http.ResponseWriter, _ *http.Request) {
	status, err := s.sender.start()
	if err != nil {
		renderWhatsAppError(w, http.StatusInternalServerError, "启动 WhatsApp 浏览器失败: "+err.Error(), "ERR_START")
		return
	}
	renderJSON(w, http.StatusOK, status)
}

func (s *whatsAppService) loginStatus(w http.ResponseWriter, _ *http.Request) {
	status := s.sender.status()
	renderJSON(w, http.StatusOK, status)
}

func (s *whatsAppService) loginLogout(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	sending := s.sending
	s.mu.Unlock()
	if sending {
		renderWhatsAppError(w, http.StatusConflict, "发送任务进行中，请先停止发送", "ERR_SENDING")
		return
	}

	if err := s.sender.close(); err != nil {
		renderWhatsAppError(w, http.StatusInternalServerError, "登出失败: "+err.Error(), "ERR_LOGOUT")
		return
	}
	renderJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *whatsAppService) contactsUpload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		renderWhatsAppError(w, http.StatusBadRequest, "请选择 CSV 文件", "ERR_NO_FILE")
		return
	}
	defer file.Close()

	if strings.ToLower(filepath.Ext(header.Filename)) != ".csv" {
		renderWhatsAppError(w, http.StatusBadRequest, "仅支持 .csv 文件", "ERR_FILE_TYPE")
		return
	}

	contacts, err := parseWhatsAppContacts(file)
	if err != nil {
		renderWhatsAppError(w, http.StatusBadRequest, "CSV 解析失败: "+err.Error(), "ERR_CSV")
		return
	}

	s.mu.Lock()
	s.contacts = contacts
	s.mu.Unlock()

	renderJSON(w, http.StatusOK, map[string]any{
		"total":    len(contacts),
		"contacts": contacts,
	})
}

func (s *whatsAppService) contactsList(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	contacts := append([]whatsAppContact(nil), s.contacts...)
	s.mu.Unlock()

	renderJSON(w, http.StatusOK, map[string]any{
		"total":    len(contacts),
		"contacts": contacts,
	})
}

func (s *whatsAppService) uploadFile(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		renderWhatsAppError(w, http.StatusBadRequest, "请选择文件", "ERR_NO_FILE")
		return
	}
	defer file.Close()

	uploaded, err := s.saveUpload(file, header)
	if err != nil {
		var httpErr whatsAppHTTPError
		if errors.As(err, &httpErr) {
			renderWhatsAppError(w, httpErr.status, httpErr.message, httpErr.code)
			return
		}
		renderWhatsAppError(w, http.StatusInternalServerError, err.Error(), "ERR_UPLOAD")
		return
	}

	s.mu.Lock()
	s.files[uploaded.id] = uploaded.file
	s.mu.Unlock()

	renderJSON(w, http.StatusOK, map[string]string{
		"id":   uploaded.id,
		"type": uploaded.file.Type,
		"name": uploaded.file.Name,
	})
}

func (s *whatsAppService) sendStart(w http.ResponseWriter, r *http.Request) {
	if !s.sendLock.TryLock() {
		renderWhatsAppError(w, http.StatusConflict, "已有发送任务在运行", "ERR_SENDING")
		return
	}

	if !s.sender.isRunning() || !s.sender.checkLoggedIn() {
		s.sendLock.Unlock()
		renderWhatsAppError(w, http.StatusBadRequest, "请先登录 WhatsApp", "ERR_NOT_LOGGED_IN")
		return
	}

	var body struct {
		ContactIDs []string          `json:"contact_ids"`
		Messages   []whatsAppMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.sendLock.Unlock()
		renderWhatsAppError(w, http.StatusBadRequest, "请求格式错误", "ERR_BAD_REQUEST")
		return
	}

	contacts, messages := s.resolveSendRequest(body.ContactIDs, body.Messages)
	if len(contacts) == 0 {
		s.sendLock.Unlock()
		renderWhatsAppError(w, http.StatusBadRequest, "未选择联系人", "ERR_NO_CONTACTS")
		return
	}
	if len(messages) == 0 {
		s.sendLock.Unlock()
		renderWhatsAppError(w, http.StatusBadRequest, "请至少添加一条消息", "ERR_NO_MESSAGES")
		return
	}

	stop := make(chan struct{})
	s.mu.Lock()
	s.stop = stop
	s.sending = true
	s.mu.Unlock()

	go s.runSend(contacts, messages, stop)
	renderJSON(w, http.StatusOK, map[string]any{"ok": true, "total": len(contacts)})
}

func (s *whatsAppService) sendStop(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.sending {
		renderWhatsAppError(w, http.StatusBadRequest, "没有正在进行的发送任务", "ERR_NOT_SENDING")
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	renderJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *whatsAppService) sendStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	sending := s.sending
	s.mu.Unlock()

	renderJSON(w, http.StatusOK, map[string]bool{"sending": sending})
}

func (s *whatsAppService) sendEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		renderWhatsAppError(w, http.StatusInternalServerError, "当前服务器不支持事件流", "ERR_SSE")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 32)
	s.mu.Lock()
	s.listeners[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.listeners, ch)
		s.mu.Unlock()
		close(ch)
	}()

	_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func (s *whatsAppService) runSend(contacts []whatsAppContact, messages []whatsAppMessage, stop <-chan struct{}) {
	defer func() {
		s.mu.Lock()
		s.sending = false
		s.mu.Unlock()
		s.sendLock.Unlock()
	}()

	if !s.sender.ensureReady() {
		s.broadcast(map[string]any{"type": "error", "error": "浏览器不可用，请重新登录"})
		return
	}

	s.sender.sendToContacts(contacts, messages, func(event map[string]any) {
		s.broadcast(event)
	}, stop)
}

func (s *whatsAppService) resolveSendRequest(ids []string, messages []whatsAppMessage) ([]whatsAppContact, []whatsAppMessage) {
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	contacts := make([]whatsAppContact, 0, len(ids))
	for _, contact := range s.contacts {
		if _, ok := selected[contact.ID]; ok {
			contacts = append(contacts, contact)
		}
	}

	resolved := make([]whatsAppMessage, 0, len(messages))
	for _, message := range messages {
		if file, ok := s.files[message.ImageID]; ok {
			message.ImagePath = file.Path
		}
		if file, ok := s.files[message.PDFID]; ok {
			message.PDFPath = file.Path
		}
		if strings.TrimSpace(message.Text) != "" || message.ImagePath != "" || message.PDFPath != "" {
			resolved = append(resolved, message)
		}
	}

	return contacts, resolved
}

func (s *whatsAppService) broadcast(data map[string]any) {
	event, err := json.Marshal(data)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for ch := range s.listeners {
		select {
		case ch <- string(event):
		default:
		}
	}
}

type savedWhatsAppUpload struct {
	id   string
	file whatsAppFile
}

func (s *whatsAppService) saveUpload(file multipart.File, header *multipart.FileHeader) (savedWhatsAppUpload, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	kind := ""
	maxSize := int64(0)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		kind = "image"
		maxSize = maxWhatsAppImageSize
	case ".pdf":
		kind = "pdf"
		maxSize = maxWhatsAppPDFSize
	default:
		return savedWhatsAppUpload{}, whatsAppHTTPError{status: http.StatusBadRequest, message: "不支持的文件类型: " + ext, code: "ERR_FILE_TYPE"}
	}

	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return savedWhatsAppUpload{}, err
	}
	if int64(len(content)) > maxSize {
		return savedWhatsAppUpload{}, whatsAppHTTPError{status: http.StatusBadRequest, message: "文件过大", code: "ERR_FILE_SIZE"}
	}

	idPrefix := "pdf"
	dirName := "pdfs"
	if kind == "image" {
		idPrefix = "img"
		dirName = "images"
	}
	id := fmt.Sprintf("%s_%s", idPrefix, uuid.NewString()[:8])
	dir := filepath.Join(s.uploadsDir, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return savedWhatsAppUpload{}, err
	}
	path := filepath.Join(dir, id+ext)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return savedWhatsAppUpload{}, err
	}
	if err := os.WriteFile(absPath, content, 0o600); err != nil {
		return savedWhatsAppUpload{}, err
	}

	return savedWhatsAppUpload{
		id: id,
		file: whatsAppFile{
			Path: absPath,
			Type: kind,
			Name: header.Filename,
		},
	}, nil
}

type whatsAppHTTPError struct {
	status  int
	message string
	code    string
}

func (e whatsAppHTTPError) Error() string {
	return e.message
}

func parseWhatsAppContacts(reader io.Reader) ([]whatsAppContact, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	headerIndex := make(map[string]int, len(rows[0]))
	for i, header := range rows[0] {
		headerIndex[strings.TrimSpace(header)] = i
	}

	contacts := make([]whatsAppContact, 0, len(rows)-1)
	for _, row := range rows[1:] {
		phone := cleanWhatsAppPhone(csvValue(row, headerIndex, "电话", "phone"))
		if phone == "" {
			continue
		}

		contacts = append(contacts, whatsAppContact{
			ID:       phone,
			ShopName: csvValue(row, headerIndex, "商家名称", "title", "name"),
			Phone:    phone,
			Address:  csvValue(row, headerIndex, "地址", "address"),
			Category: csvValue(row, headerIndex, "分类", "category"),
			Rating:   csvValue(row, headerIndex, "评分", "review_rating", "rating"),
			Email:    csvValue(row, headerIndex, "邮箱", "emails", "email"),
			Website:  csvValue(row, headerIndex, "官网", "web_site", "website"),
			Selected: true,
		})
	}

	return contacts, nil
}

func csvValue(row []string, headerIndex map[string]int, names ...string) string {
	for _, name := range names {
		if idx, ok := headerIndex[name]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}

	return ""
}

func cleanWhatsAppPhone(phone string) string {
	return phoneCleaner.ReplaceAllString(strings.TrimSpace(phone), "")
}

func renderWhatsAppError(w http.ResponseWriter, status int, message, code string) {
	renderJSON(w, status, map[string]any{
		"detail": map[string]string{
			"error": message,
			"code":  code,
		},
	})
}

type whatsAppSender struct {
	sessionDir string
	debugDir   string

	mu         sync.Mutex
	playwright *playwright.Playwright
	browser    playwright.BrowserContext
	page       playwright.Page
}

func newWhatsAppSender(sessionDir, debugDir string) *whatsAppSender {
	return &whatsAppSender{sessionDir: sessionDir, debugDir: debugDir}
}

func (s *whatsAppSender) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.page != nil && !s.page.IsClosed()
}

func (s *whatsAppSender) start() (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page != nil && !s.page.IsClosed() {
		return map[string]any{"logged_in": s.checkLoggedInLocked(), "phone": nil}, nil
	}

	if err := s.startLocked(); err != nil {
		return nil, err
	}

	return map[string]any{"logged_in": s.checkLoggedInLocked(), "phone": nil}, nil
}

func (s *whatsAppSender) status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page == nil || s.page.IsClosed() {
		return map[string]any{"logged_in": false, "phone": nil}
	}

	return map[string]any{"logged_in": s.checkLoggedInLocked(), "phone": nil}
}

func (s *whatsAppSender) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	if s.browser != nil {
		errs = append(errs, s.browser.Close())
	}
	if s.playwright != nil {
		errs = append(errs, s.playwright.Stop())
	}
	s.browser = nil
	s.page = nil
	s.playwright = nil

	return errors.Join(errs...)
}

func (s *whatsAppSender) checkLoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.checkLoggedInLocked()
}

func (s *whatsAppSender) ensureReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page != nil && !s.page.IsClosed() {
		return true
	}
	if err := s.closeLocked(); err != nil {
		log.Printf("close stale WhatsApp browser: %v", err)
	}
	if err := s.startLocked(); err != nil {
		log.Printf("restart WhatsApp browser: %v", err)
		return false
	}
	time.Sleep(5 * time.Second)

	return s.checkLoggedInLocked()
}

func (s *whatsAppSender) startLocked() error {
	absSessionDir, err := filepath.Abs(s.sessionDir)
	if err != nil {
		return fmt.Errorf("failed to resolve session dir: %w", err)
	}
	s.sessionDir = absSessionDir

	if err := os.MkdirAll(s.sessionDir, 0o755); err != nil {
		return err
	}

	pw, err := playwright.Run()
	if err != nil {
		return err
	}

	headless := strings.EqualFold(os.Getenv("WHATSAPP_HEADLESS"), "1") ||
		strings.EqualFold(os.Getenv("WHATSAPP_HEADLESS"), "true")
	ctx, err := pw.Chromium.LaunchPersistentContext(s.sessionDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: &headless,
		Viewport: &playwright.Size{
			Width:  1280,
			Height: 800,
		},
		Args: []string{"--disable-blink-features=AutomationControlled"},
	})
	if err != nil {
		_ = pw.Stop()
		return err
	}

	pages := ctx.Pages()
	if len(pages) > 0 {
		s.page = pages[0]
	} else {
		s.page, err = ctx.NewPage()
		if err != nil {
			_ = ctx.Close()
			_ = pw.Stop()
			return err
		}
	}

	if _, err := s.page.Goto("https://web.whatsapp.com", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		_ = ctx.Close()
		_ = pw.Stop()
		return err
	}

	s.playwright = pw
	s.browser = ctx

	return nil
}

func (s *whatsAppSender) closeLocked() error {
	var errs []error
	if s.browser != nil {
		errs = append(errs, s.browser.Close())
	}
	if s.playwright != nil {
		errs = append(errs, s.playwright.Stop())
	}
	s.browser = nil
	s.page = nil
	s.playwright = nil

	return errors.Join(errs...)
}

func (s *whatsAppSender) checkLoggedInLocked() bool {
	if s.page == nil || s.page.IsClosed() {
		return false
	}

	selectors := append([]string{}, whatsAppSelectors["logged_in"]...)
	selectors = append(selectors, whatsAppSelectors["chat_input"]...)
	for _, selector := range selectors {
		timeout := 2000.0
		if _, err := s.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: &timeout}); err == nil {
			return true
		}
	}

	return false
}

func (s *whatsAppSender) sendToContacts(contacts []whatsAppContact, messages []whatsAppMessage, progress func(map[string]any), stop <-chan struct{}) {
	total := len(contacts)
	success := 0
	failed := 0

	for i, contact := range contacts {
		select {
		case <-stop:
			progress(map[string]any{"type": "complete", "success": success, "failed": failed, "total": total})
			return
		default:
		}

		lastError := "发送失败或号码未注册"
		contactOK := s.openChat(contact.Phone)
		if !contactOK {
			failed++
			progress(map[string]any{"type": "error", "contact": contact.displayName(), "error": "号码未注册或无法打开聊天", "current": i + 1, "total": total})
			continue
		}

		for _, message := range messages {
			select {
			case <-stop:
				contactOK = false
			default:
			}
			if !contactOK {
				break
			}

			text := renderWhatsAppMessage(message.Text, contact)
			if ok := s.sendInCurrentChat(text, message.ImagePath, message.PDFPath); !ok {
				contactOK = false
				lastError = "发送失败"
				break
			}

			// Sleep with early stop check
			select {
			case <-stop:
				contactOK = false
			case <-time.After(time.Duration(5+(i%3)*2) * time.Second):
			}
		}

		if contactOK {
			success++
			progress(map[string]any{"type": "progress", "contact": contact.displayName(), "status": "success", "current": i + 1, "total": total})
		} else {
			failed++
			progress(map[string]any{"type": "error", "contact": contact.displayName(), "error": lastError, "current": i + 1, "total": total})
		}
	}

	progress(map[string]any{"type": "complete", "success": success, "failed": failed, "total": total})
}

func (c whatsAppContact) displayName() string {
	if c.ShopName != "" {
		return c.ShopName
	}

	return c.Phone
}

func (s *whatsAppSender) dismissPopupsLocked() {
	for _, selector := range whatsAppSelectors["dismiss_popup"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err == nil && count > 0 {
			_ = loc.First().Click()
			log.Printf("[whatsapp] dismissed popup with selector: %s", selector)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (s *whatsAppSender) openChat(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page == nil || s.page.IsClosed() {
		return false
	}

	target := "https://web.whatsapp.com/send?phone=" + url.QueryEscape(phone)
	if _, err := s.page.Goto(target, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		return false
	}

	return s.waitForChatOrInvalidPhoneLocked(15 * time.Second)
}

func (s *whatsAppSender) waitForChatLocked(timeout time.Duration) bool {
	if s.dismissInvalidPhonePopupLocked() {
		return false
	}
	return s.hasChatInputLocked(timeout)
}

func (s *whatsAppSender) hasChatInputLocked(timeout time.Duration) bool {
	for _, selector := range whatsAppSelectors["chat_input"] {
		ms := float64(timeout / time.Millisecond)
		if _, err := s.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: &ms}); err == nil {
			return true
		}
	}

	return false
}

func (s *whatsAppSender) dismissInvalidPhonePopupLocked() bool {
	for _, selector := range whatsAppSelectors["invalid_phone_popup"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err == nil && count > 0 {
			log.Printf("[whatsapp] detected invalid or unregistered phone popup with selector: %s", selector)
			s.dismissPopupsLocked()
			return true
		}
	}

	return false
}

func (s *whatsAppSender) waitForChatOrInvalidPhoneLocked(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.dismissInvalidPhonePopupLocked() {
			return false
		}
		if s.hasChatInputLocked(500 * time.Millisecond) {
			time.Sleep(2 * time.Second)
			return true
		}
		s.dismissPopupsLocked()
		time.Sleep(300 * time.Millisecond)
	}

	return false
}

func (s *whatsAppSender) sendInCurrentChat(message, imagePath, pdfPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[whatsapp] sendInCurrentChat: image=%q pdf=%q", imagePath, pdfPath)
	hasImage := imagePath != "" && fileExists(imagePath)
	hasPDF := pdfPath != "" && fileExists(pdfPath)
	log.Printf("[whatsapp] hasImage=%v hasPDF=%v", hasImage, hasPDF)
	if !hasImage && !hasPDF {
		return s.sendTextInChatLocked(message)
	}

	if hasImage {
		ok := s.attachImageLocked(imagePath, message)
		if !ok && message != "" {
			return s.sendTextInChatLocked(message)
		}
		if !ok {
			return false
		}
	}

	if hasPDF {
		log.Printf("[whatsapp] attaching PDF: %s", pdfPath)
		ok := s.attachDocumentLocked(pdfPath)
		log.Printf("[whatsapp] attachDocument result: %v", ok)
		if !ok && !hasImage && message != "" {
			return s.sendTextInChatLocked(message)
		}
		if !ok && !hasImage {
			return false
		}
		if message != "" && !hasImage {
			time.Sleep(2 * time.Second)
			return s.sendTextInChatLocked(message)
		}
	}

	return true
}

func (s *whatsAppSender) sendTextInChatLocked(message string) bool {
	if s.page == nil || s.page.IsClosed() || message == "" {
		return false
	}

	for _, selector := range whatsAppSelectors["chat_input"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		if err := loc.First().Click(); err != nil {
			continue
		}
		delay := 30.0
		if err := s.page.Keyboard().Type(message, playwright.KeyboardTypeOptions{Delay: &delay}); err != nil {
			continue
		}
		time.Sleep(time.Second)
		return s.page.Keyboard().Press("Enter") == nil
	}

	return false
}

func (s *whatsAppSender) attachImageLocked(path, caption string) bool {
	if s.tryDirectUploadLocked(path, []string{
		`input[accept="image/*,video/mp4,video/3gpp,video/quicktime"]`,
		`input[accept*="image"]`,
	}) {
		time.Sleep(3 * time.Second)
		return s.sendPreviewLocked(caption)
	}

	if !s.findAndClickLocked("attach_button", 5*time.Second) {
		s.debugScreenshotLocked("attach_button_not_found")
		return false
	}
	time.Sleep(time.Second)

	fc, err := s.page.ExpectFileChooser(func() error {
		for _, selector := range whatsAppSelectors["attach_image_option"] {
			loc := s.page.Locator(selector)
			count, err := loc.Count()
			if err == nil && count > 0 && loc.First().Click() == nil {
				return nil
			}
		}
		return fmt.Errorf("未找到图片上传选项")
	})
	if err != nil {
		s.debugScreenshotLocked("file_chooser_failed")
		return false
	}
	if err := fc.SetFiles(path); err != nil {
		return false
	}
	time.Sleep(3 * time.Second)

	return s.sendPreviewLocked(caption)
}

func (s *whatsAppSender) attachDocumentLocked(path string) bool {
	log.Printf("[whatsapp] attachDocumentLocked: path=%s", path)

	// Strategy 1: Direct upload via hidden file input with accept="*" and multiple attribute.
	// WhatsApp Web has two hidden <input type="file"> elements:
	//   - first: accept="image/*" (for photos/videos)
	//   - second: accept="*" multiple (for documents/PDFs)
	if s.tryDirectUploadLocked(path, []string{
		`input[type="file"][multiple]`,
		`input[accept="*"]`,
		`input[accept*="pdf"]`,
	}) {
		log.Printf("[whatsapp] direct PDF upload succeeded")
		time.Sleep(2 * time.Second)
		return s.clickSendButtonLocked()
	}

	log.Printf("[whatsapp] direct upload failed, trying attach button flow")

	// Strategy 2: Click attach button, then click "Document" option in menu.
	if !s.findAndClickLocked("attach_button", 5*time.Second) {
		log.Printf("[whatsapp] attach button not found")
		s.debugScreenshotLocked("doc_attach_not_found")
		return false
	}
	log.Printf("[whatsapp] attach button clicked, waiting for menu")
	time.Sleep(time.Second)

	fc, err := s.page.ExpectFileChooser(func() error {
		for _, selector := range whatsAppSelectors["attach_document_option"] {
			log.Printf("[whatsapp] trying doc selector: %s", selector)
			loc := s.page.Locator(selector)
			count, err := loc.Count()
			if err == nil && count > 0 && loc.First().Click() == nil {
				log.Printf("[whatsapp] doc selector matched: %s", selector)
				return nil
			}
		}
		return fmt.Errorf("未找到文档上传选项")
	})
	if err != nil {
		log.Printf("[whatsapp] file chooser failed: %v", err)
		s.debugScreenshotLocked("doc_file_chooser_failed")
		return false
	}
	if err := fc.SetFiles(path); err != nil {
		return false
	}
	time.Sleep(2 * time.Second)

	return s.clickSendButtonLocked()
}

func (s *whatsAppSender) tryDirectUploadLocked(path string, selectors []string) bool {
	for _, selector := range selectors {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err != nil || count == 0 {
			log.Printf("[whatsapp] direct upload selector not found: %s (count=%d err=%v)", selector, count, err)
			continue
		}
		log.Printf("[whatsapp] direct upload selector found: %s, setting files", selector)
		if err := loc.First().SetInputFiles(path); err == nil {
			return true
		} else {
			log.Printf("[whatsapp] SetInputFiles failed for %s: %v", selector, err)
		}
	}

	return false
}

func (s *whatsAppSender) sendPreviewLocked(caption string) bool {
	if caption != "" {
		for _, selector := range []string{`div[contenteditable="true"][data-tab="10"]`, `div[contenteditable="true"]`} {
			loc := s.page.Locator(selector)
			count, err := loc.Count()
			if err != nil || count == 0 {
				continue
			}
			if err := loc.Last().Click(); err != nil {
				continue
			}
			delay := 30.0
			_ = s.page.Keyboard().Type(caption, playwright.KeyboardTypeOptions{Delay: &delay})
			break
		}
		time.Sleep(time.Second)
	}

	for _, selector := range whatsAppSelectors["preview_send"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err == nil && count > 0 && loc.First().Click() == nil {
			return true
		}
	}

	s.debugScreenshotLocked("send_button_not_found")

	return s.page.Keyboard().Press("Enter") == nil
}

func (s *whatsAppSender) clickSendButtonLocked() bool {
	for _, selector := range whatsAppSelectors["send_button"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		if strings.Contains(selector, "span") {
			if _, err := loc.First().Evaluate(`el => el.closest('button').click()`, nil); err == nil {
				return true
			}
			continue
		}
		timeout := 5000.0
		if err := loc.First().Click(playwright.LocatorClickOptions{Timeout: &timeout}); err == nil {
			return true
		}
	}

	return s.page.Keyboard().Press("Enter") == nil
}

func (s *whatsAppSender) findAndClickLocked(selectorKey string, timeout time.Duration) bool {
	ms := float64(timeout / time.Millisecond)
	for _, selector := range whatsAppSelectors[selectorKey] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		if err := loc.First().Click(playwright.LocatorClickOptions{Timeout: &ms}); err == nil {
			return true
		}
	}

	return false
}

func (s *whatsAppSender) debugScreenshotLocked(tag string) {
	if s.page == nil || s.page.IsClosed() {
		return
	}
	if err := os.MkdirAll(s.debugDir, 0o755); err != nil {
		return
	}

	_, _ = s.page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(filepath.Join(s.debugDir, tag+".png")),
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func renderWhatsAppMessage(template string, contact whatsAppContact) string {
	replacements := map[string]string{
		"{name}":     contact.ShopName,
		"{phone}":    contact.Phone,
		"{address}":  contact.Address,
		"{category}": contact.Category,
		"{rating}":   contact.Rating,
		"{email}":    contact.Email,
	}
	result := template
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}

	return result
}

var whatsAppSelectors = map[string][]string{
	"dismiss_popup": {
		`div[role="dialog"] button`,
		`[data-animate-modal-popup="true"] button`,
		`[data-animate-modal-backdrop="true"] button`,
		`div[role="alertdialog"] button`,
		`span[data-icon="x"]`,
		`button[aria-label="关闭"]`,
		`button[aria-label="Close"]`,
		`button[aria-label="好的"]`,
		`button[aria-label="确定"]`,
		`button[aria-label="OK"]`,
	},
	"invalid_phone_popup": {
		`//*[(@role="dialog" or @role="alertdialog") and (contains(., "未注册") or contains(., "无效") or contains(., "不是有效") or contains(., "无法使用") or contains(., "not registered") or contains(., "not valid") or contains(., "invalid"))]`,
		`//*[contains(., "Phone number shared via url is invalid")]`,
		`//*[contains(., "The phone number shared via url is invalid")]`,
	},
	"chat_input": {
		`div[contenteditable="true"][data-tab="10"]`,
		`footer div[contenteditable="true"]`,
		`div[contenteditable="true"][title="输入消息"]`,
		`div[contenteditable="true"][title="Type a message"]`,
		`div[contenteditable="true"][data-placeholder]`,
		`div[contenteditable="true"][aria-placeholder]`,
	},
	"logged_in": {
		`#side`,
		`[data-testid="chat-list"]`,
		`div[aria-label="Chat list"]`,
		`div[aria-label="聊天列表"]`,
		`div[role="grid"][aria-label="Chat list"]`,
		`div[role="grid"][aria-label="聊天列表"]`,
		`div[contenteditable="true"][data-tab="3"]`,
		`div[contenteditable="true"][title="搜索或开始新对话"]`,
		`div[contenteditable="true"][title="Search input textbox"]`,
	},
	"send_button": {
		`span[data-icon="wds-ic-send-filled"]`,
		`footer button[data-tab="11"]`,
		`button[data-tab="11"]`,
		`span[data-icon="send"]`,
		`span[data-icon="send-v2"]`,
		`div[aria-label="发送"]`,
		`div[aria-label="Send"]`,
		`button[aria-label="发送"]`,
		`button[aria-label="Send"]`,
		`[data-icon="send"]`,
	},
	"attach_button": {
		`span[data-icon="plus-rounded"]`,
		`span[data-icon="clip"]`,
		`[aria-label="附件"]`,
		`[aria-label="Attach"]`,
		`span[data-icon="plus"]`,
		`span[data-icon="plus-filled"]`,
		`[data-icon="plus"]`,
		`[data-testid="attach-menu-btn"]`,
		`#main footer [data-icon="plus"]`,
	},
	"attach_image_option": {
		`span[data-testid="attach-image"]`,
		`input[accept="image/*,video/mp4,video/3gpp,video/quicktime"]`,
		`input[accept*="image"]`,
		`[aria-label="照片和视频"]`,
		`[aria-label="Photos & videos"]`,
		`span[data-icon="image"]`,
	},
	"attach_document_option": {
		`//span[text()='Document']`,
		`//span[text()='文档']`,
		`//div[@role='application']/ul//span[text()='Document']`,
		`span[data-testid="attach-document"]`,
		`span[data-icon="doc"]`,
		`[aria-label="文档"]`,
		`[aria-label="Document"]`,
	},
	"preview_send": {
		`span[data-icon="wds-ic-send-filled"]`,
		`span[data-icon="send"]`,
		`span[data-icon="send-v2"]`,
		`div[aria-label="发送"]`,
		`div[aria-label="Send"]`,
		`button[aria-label="发送"]`,
		`button[aria-label="Send"]`,
		`[data-icon="send"]`,
	},
}
