package whatsapp

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Sender manages a WhatsApp Web browser session via Playwright.
type Sender struct {
	sessionDir string
	debugDir   string

	mu         sync.Mutex
	playwright *playwright.Playwright
	browser    playwright.BrowserContext
	page       playwright.Page
}

// NewSender creates a new Sender with the given session and debug directories.
func NewSender(sessionDir, debugDir string) *Sender {
	return &Sender{sessionDir: sessionDir, debugDir: debugDir}
}

// IsRunning reports whether the browser session is active.
func (s *Sender) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.page != nil && !s.page.IsClosed()
}

// Start launches the browser and navigates to WhatsApp Web.
// Returns the login status if the session was already active.
func (s *Sender) Start() (map[string]any, error) {
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

// Status returns the current login status of the WhatsApp session.
func (s *Sender) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page == nil || s.page.IsClosed() {
		return map[string]any{"logged_in": false, "phone": nil}
	}

	return map[string]any{"logged_in": s.checkLoggedInLocked(), "phone": nil}
}

// Close shuts down the browser and Playwright instance.
func (s *Sender) Close() error {
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

// CheckLoggedIn reports whether the user is logged in to WhatsApp Web.
func (s *Sender) CheckLoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.checkLoggedInLocked()
}

// EnsureReady ensures the browser session is active and the user is logged in,
// restarting if necessary.
func (s *Sender) EnsureReady() bool {
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

// SendToContacts sends messages to a list of contacts, calling progress for each event.
func (s *Sender) SendToContacts(contacts []Contact, messages []Message, progress func(map[string]any), stop <-chan struct{}) {
	s.SendToContactsWithOptions(contacts, messages, DefaultSendOptions(), progress, stop)
}

// SendToContactsWithOptions sends messages with configurable compliant pacing and safety pauses.
func (s *Sender) SendToContactsWithOptions(contacts []Contact, messages []Message, options SendOptions, progress func(map[string]any), stop <-chan struct{}) {
	options = options.Normalize()
	total := len(contacts)
	success := 0
	failed := 0
	consecutiveFailures := 0

	for i, contact := range contacts {
		select {
		case <-stop:
			progress(map[string]any{"type": "complete", "success": success, "failed": failed, "total": total})
			return
		default:
		}

		lastError := "send failed or number not registered"
		contactOK := s.OpenChat(contact.Phone)
		if !contactOK {
			failed++
			consecutiveFailures++
			progress(map[string]any{"type": "error", "contact": contact.DisplayName(), "error": "号码未注册或无法打开聊天，已跳过", "current": i + 1, "total": total})
		} else {
			for _, message := range messages {
				select {
				case <-stop:
					contactOK = false
				default:
				}
				if !contactOK {
					break
				}

				text := RenderMessage(message.Text, contact)
				if ok := s.SendInCurrentChat(text, message.ImagePath, message.PDFPath); !ok {
					contactOK = false
					lastError = "send failed"
					break
				}
			}

			if contactOK {
				success++
				consecutiveFailures = 0
				progress(map[string]any{"type": "progress", "contact": contact.DisplayName(), "status": "success", "current": i + 1, "total": total})
			} else {
				failed++
				consecutiveFailures++
				progress(map[string]any{"type": "error", "contact": contact.DisplayName(), "error": lastError, "current": i + 1, "total": total})
			}
		}

		if consecutiveFailures >= options.MaxConsecutiveFailures {
			progress(map[string]any{
				"type":    "paused",
				"reason":  fmt.Sprintf("连续失败 %d 次，已自动暂停", consecutiveFailures),
				"success": success,
				"failed":  failed,
				"current": i + 1,
				"total":   total,
			})
			return
		}

		if i+1 >= total {
			continue
		}
		waitSeconds := randomSeconds(options.ContactDelayMinSeconds, options.ContactDelayMaxSeconds)
		waitReason := "联系人间隔"
		if options.BatchSize > 0 && (i+1)%options.BatchSize == 0 {
			waitSeconds = randomSeconds(options.BatchDelayMinSeconds, options.BatchDelayMaxSeconds)
			waitReason = fmt.Sprintf("已发送 %d 人，批次休息", i+1)
		}
		progress(map[string]any{
			"type":    "wait",
			"reason":  waitReason,
			"seconds": waitSeconds,
			"current": i + 1,
			"total":   total,
		})
		if !waitWithStop(time.Duration(waitSeconds)*time.Second, stop) {
			progress(map[string]any{"type": "complete", "success": success, "failed": failed, "total": total})
			return
		}
	}

	progress(map[string]any{"type": "complete", "success": success, "failed": failed, "total": total})
}

func randomSeconds(minSeconds, maxSeconds int) int {
	if minSeconds >= maxSeconds {
		return minSeconds
	}
	return minSeconds + rand.Intn(maxSeconds-minSeconds+1)
}

func waitWithStop(duration time.Duration, stop <-chan struct{}) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}

// OpenChat navigates to a WhatsApp chat for the given phone number.
func (s *Sender) OpenChat(phone string) bool {
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

// SendInCurrentChat sends a message with optional image and PDF in the currently open chat.
func (s *Sender) SendInCurrentChat(message, imagePath, pdfPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[whatsapp] SendInCurrentChat: image=%q pdf=%q", imagePath, pdfPath)
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

// SendTextInChat types and sends a text message in the currently open chat.
func (s *Sender) SendTextInChat(message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendTextInChatLocked(message)
}

// AttachImage attaches an image file to the current chat with an optional caption.
func (s *Sender) AttachImage(path, caption string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attachImageLocked(path, caption)
}

// AttachDocument attaches a PDF document to the current chat.
func (s *Sender) AttachDocument(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attachDocumentLocked(path)
}

// DismissPopups attempts to close any popup dialogs currently visible.
func (s *Sender) DismissPopups() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dismissPopupsLocked()
}

// RenderMessage replaces template placeholders with contact field values.
func RenderMessage(template string, contact Contact) string {
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

func (s *Sender) startLocked() error {
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

func (s *Sender) closeLocked() error {
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

func (s *Sender) checkLoggedInLocked() bool {
	if s.page == nil || s.page.IsClosed() {
		return false
	}

	for _, selector := range append(append([]string{}, Selectors["logged_in"]...), Selectors["chat_input"]...) {
		timeout := 2000.0
		if _, err := s.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: &timeout}); err == nil {
			return true
		}
	}

	return false
}

func (s *Sender) dismissPopupsLocked() {
	for _, selector := range Selectors["dismiss_popup"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err == nil && count > 0 {
			_ = loc.First().Click()
			log.Printf("[whatsapp] dismissed popup with selector: %s", selector)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (s *Sender) dismissInvalidPhonePopupLocked() bool {
	for _, selector := range Selectors["invalid_phone_popup"] {
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

func (s *Sender) waitForChatOrInvalidPhoneLocked(timeout time.Duration) bool {
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

func (s *Sender) waitForChatLocked(timeout time.Duration) bool {
	if s.dismissInvalidPhonePopupLocked() {
		return false
	}
	return s.hasChatInputLocked(timeout)
}

func (s *Sender) hasChatInputLocked(timeout time.Duration) bool {
	for _, selector := range Selectors["chat_input"] {
		ms := float64(timeout / time.Millisecond)
		if _, err := s.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{Timeout: &ms}); err == nil {
			return true
		}
	}

	return false
}

func (s *Sender) sendTextInChatLocked(message string) bool {
	if s.page == nil || s.page.IsClosed() || message == "" {
		return false
	}

	for _, selector := range Selectors["chat_input"] {
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

func (s *Sender) attachImageLocked(path, caption string) bool {
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
		for _, selector := range Selectors["attach_image_option"] {
			loc := s.page.Locator(selector)
			count, err := loc.Count()
			if err == nil && count > 0 && loc.First().Click() == nil {
				return nil
			}
		}
		return fmt.Errorf("image upload option not found")
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

func (s *Sender) attachDocumentLocked(path string) bool {
	log.Printf("[whatsapp] attachDocumentLocked: path=%s", path)

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

	if !s.findAndClickLocked("attach_button", 5*time.Second) {
		log.Printf("[whatsapp] attach button not found")
		s.debugScreenshotLocked("doc_attach_not_found")
		return false
	}
	log.Printf("[whatsapp] attach button clicked, waiting for menu")
	time.Sleep(time.Second)

	fc, err := s.page.ExpectFileChooser(func() error {
		for _, selector := range Selectors["attach_document_option"] {
			log.Printf("[whatsapp] trying doc selector: %s", selector)
			loc := s.page.Locator(selector)
			if s.clickDocumentOptionLocatorLocked(loc) {
				log.Printf("[whatsapp] doc selector matched: %s", selector)
				return nil
			}
		}
		return fmt.Errorf("document upload option not found")
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

func (s *Sender) clickDocumentOptionLocatorLocked(loc playwright.Locator) bool {
	count, err := loc.Count()
	if err != nil || count == 0 {
		return false
	}

	first := loc.First()
	timeout := 5000.0
	if _, err := first.Evaluate(`el => {
		const target = el.closest('li, [role="button"], button, div[tabindex], div');
		if (!target) return false;
		target.click();
		return true;
	}`, nil); err == nil {
		return true
	}

	return first.Click(playwright.LocatorClickOptions{Timeout: &timeout}) == nil
}

func (s *Sender) tryDirectUploadLocked(path string, selectors []string) bool {
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

func (s *Sender) sendPreviewLocked(caption string) bool {
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

	for _, selector := range Selectors["preview_send"] {
		loc := s.page.Locator(selector)
		count, err := loc.Count()
		if err == nil && count > 0 && loc.First().Click() == nil {
			return true
		}
	}

	s.debugScreenshotLocked("send_button_not_found")

	return s.page.Keyboard().Press("Enter") == nil
}

func (s *Sender) clickSendButtonLocked() bool {
	for _, selector := range Selectors["send_button"] {
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

func (s *Sender) findAndClickLocked(selectorKey string, timeout time.Duration) bool {
	ms := float64(timeout / time.Millisecond)
	for _, selector := range Selectors[selectorKey] {
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

func (s *Sender) debugScreenshotLocked(tag string) {
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
