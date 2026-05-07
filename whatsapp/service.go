package whatsapp

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// OnProgress is called with event data whenever a progress event occurs during sending.
// Callers (web server, Wails app) set this to receive real-time updates.
type OnProgress func(event map[string]any)

// Service orchestrates WhatsApp contact management, file uploads, and message sending.
type Service struct {
	sender     *Sender
	contacts   []Contact
	files      map[string]File
	uploadsDir string

	mu         sync.Mutex
	sendLock   sync.Mutex
	stop       chan struct{}
	sending    bool
	listener   *Listener
	OnProgress OnProgress
}

// NewService creates a new WhatsApp service rooted at dataFolder/whatsapp.
func NewService(dataFolder string) *Service {
	base := filepath.Join(dataFolder, "whatsapp")
	uploads := filepath.Join(base, "uploads")

	return &Service{
		sender:     NewSender(filepath.Join(base, "session"), filepath.Join(base, "debug_screenshots")),
		files:      make(map[string]File),
		uploadsDir: uploads,
		stop:       make(chan struct{}),
	}
}

// Close shuts down the WhatsApp sender.
func (s *Service) Close() {
	if s == nil {
		return
	}
	if err := s.sender.Close(); err != nil {
		log.Printf("close WhatsApp sender: %v", err)
	}
}

// LoginStart launches the WhatsApp Web browser and returns login status.
func (s *Service) LoginStart() (map[string]any, error) {
	status, err := s.sender.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start WhatsApp browser: %w", err)
	}
	return status, nil
}

// LoginStatus returns the current login status of the WhatsApp session.
func (s *Service) LoginStatus() map[string]any {
	return s.sender.Status()
}

// Logout closes the WhatsApp browser session. Returns an error if a send is in progress.
func (s *Service) Logout() error {
	s.mu.Lock()
	sending := s.sending
	s.mu.Unlock()
	if sending {
		return HTTPError{Status: 409, Message: "send task in progress, please stop sending first", Code: "ERR_SENDING"}
	}

	if err := s.sender.Close(); err != nil {
		return fmt.Errorf("logout failed: %w", err)
	}
	return nil
}

// UploadContacts parses CSV data and stores the contacts.
func (s *Service) UploadContacts(csvData []byte) (map[string]any, error) {
	contacts, err := ParseContacts(strings.NewReader(string(csvData)))
	if err != nil {
		return nil, fmt.Errorf("CSV parse failed: %w", err)
	}

	s.mu.Lock()
	s.contacts = contacts
	s.mu.Unlock()

	return map[string]any{
		"total":    len(contacts),
		"contacts": contacts,
	}, nil
}

// ListContacts returns a copy of the current contact list.
func (s *Service) ListContacts() map[string]any {
	s.mu.Lock()
	contacts := append([]Contact(nil), s.contacts...)
	s.mu.Unlock()

	return map[string]any{
		"total":    len(contacts),
		"contacts": contacts,
	}
}

// UploadFile saves an uploaded file and returns its ID, type, and name.
func (s *Service) UploadFile(fileData []byte, fileName string) (map[string]string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	kind := ""
	maxSize := int64(0)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		kind = "image"
		maxSize = MaxImageSize
	case ".pdf":
		kind = "pdf"
		maxSize = MaxPDFSize
	default:
		return nil, HTTPError{Status: 400, Message: "unsupported file type: " + ext, Code: "ERR_FILE_TYPE"}
	}

	if int64(len(fileData)) > maxSize {
		return nil, HTTPError{Status: 400, Message: "file too large", Code: "ERR_FILE_SIZE"}
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
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}
	path := filepath.Join(dir, id+ext)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve upload path: %w", err)
	}
	if err := os.WriteFile(absPath, fileData, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write upload file: %w", err)
	}

	s.mu.Lock()
	s.files[id] = File{Path: absPath, Type: kind, Name: fileName}
	s.mu.Unlock()

	return map[string]string{
		"id":   id,
		"type": kind,
		"name": fileName,
	}, nil
}

// SendStart begins sending messages to the specified contacts. Returns an error if
// already sending, not logged in, no contacts, or no messages.
func (s *Service) SendStart(contactIDs []string, messages []Message, options SendOptions) error {
	if !s.sendLock.TryLock() {
		return HTTPError{Status: 409, Message: "a send task is already running", Code: "ERR_SENDING"}
	}

	if !s.sender.IsRunning() || !s.sender.CheckLoggedIn() {
		s.sendLock.Unlock()
		return HTTPError{Status: 400, Message: "please log in to WhatsApp first", Code: "ERR_NOT_LOGGED_IN"}
	}

	contacts, resolved := s.resolveSendRequest(contactIDs, messages)
	if len(contacts) == 0 {
		s.sendLock.Unlock()
		return HTTPError{Status: 400, Message: "no contacts selected", Code: "ERR_NO_CONTACTS"}
	}
	if len(resolved) == 0 {
		s.sendLock.Unlock()
		return HTTPError{Status: 400, Message: "please add at least one message", Code: "ERR_NO_MESSAGES"}
	}

	stop := make(chan struct{})
	s.mu.Lock()
	s.stop = stop
	s.sending = true
	s.mu.Unlock()

	go s.runSend(contacts, resolved, options.Normalize(), stop)
	return nil
}

// SendStop stops the current send task.
func (s *Service) SendStop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.sending {
		return HTTPError{Status: 400, Message: "no send task in progress", Code: "ERR_NOT_SENDING"}
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	return nil
}

// SendStatus returns whether a send task is currently running.
func (s *Service) SendStatus() map[string]bool {
	s.mu.Lock()
	sending := s.sending
	s.mu.Unlock()

	return map[string]bool{"sending": sending}
}

func (s *Service) runSend(contacts []Contact, messages []Message, options SendOptions, stop <-chan struct{}) {
	defer func() {
		s.mu.Lock()
		s.sending = false
		s.mu.Unlock()
		s.sendLock.Unlock()
	}()

	if !s.sender.EnsureReady() {
		s.broadcast(map[string]any{"type": "error", "error": "browser unavailable, please log in again"})
		return
	}

	s.sender.SendToContactsWithOptions(contacts, messages, options, func(event map[string]any) {
		s.broadcast(event)
	}, stop)
}

func (s *Service) resolveSendRequest(ids []string, messages []Message) ([]Contact, []Message) {
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	contacts := make([]Contact, 0, len(ids))
	for _, contact := range s.contacts {
		if _, ok := selected[contact.ID]; ok {
			contacts = append(contacts, contact)
		}
	}

	resolved := make([]Message, 0, len(messages))
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

func (s *Service) broadcast(data map[string]any) {
	if s.OnProgress != nil {
		s.OnProgress(data)
		return
	}

	// Fallback: marshal to JSON for any listener-based consumers.
	_, _ = json.Marshal(data)
}

// Sender returns the underlying WhatsApp Sender for direct browser interaction.
func (s *Service) Sender() *Sender {
	return s.sender
}

// SetAgentListener sets or clears the agent listener. When set, the listener
// polls WhatsApp Web for incoming messages.
func (s *Service) SetAgentListener(l *Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		s.listener.Stop()
	}
	s.listener = l
}
