package whatsapp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// MessageHandler is called when a new incoming message is detected.
type MessageHandler func(msg IncomingMessage)

// Listener polls WhatsApp Web for incoming messages via the same Playwright page
// used by Sender. It shares the Sender's mutex to avoid concurrent DOM access.
type Listener struct {
	sender *Sender

	mu       sync.Mutex
	running  bool
	stop     chan struct{}
	handlers []MessageHandler

	// lastSeen tracks the last message timestamp per phone to avoid duplicates.
	lastSeen map[string]time.Time
}

// NewListener creates a Listener that reads from the Sender's Playwright page.
func NewListener(sender *Sender) *Listener {
	return &Listener{
		sender:   sender,
		lastSeen: make(map[string]time.Time),
	}
}

// Start begins polling for incoming messages at the given interval.
func (l *Listener) Start(interval time.Duration) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return fmt.Errorf("listener already running")
	}
	l.stop = make(chan struct{})
	l.running = true
	l.mu.Unlock()

	go l.pollLoop(interval)
	log.Printf("[whatsapp-listener] started with interval %v", interval)
	return nil
}

// Stop stops the listener.
func (l *Listener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.running {
		return
	}
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
	l.running = false
}

// OnMessage registers a handler for incoming messages.
func (l *Listener) OnMessage(handler MessageHandler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers = append(l.handlers, handler)
}

// IsRunning reports whether the listener is currently active.
func (l *Listener) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

func (l *Listener) pollLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stop:
			log.Printf("[whatsapp-listener] stopped")
			return
		case <-ticker.C:
			messages := l.pollOnce()
			for _, msg := range messages {
				l.dispatch(msg)
			}
		}
	}
}

func (l *Listener) dispatch(msg IncomingMessage) {
	l.mu.Lock()
	handlers := append([]MessageHandler(nil), l.handlers...)
	l.mu.Unlock()

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[whatsapp-listener] handler panic: %v", r)
				}
			}()
			h(msg)
		}()
	}
}

// pollOnce scans the chat list for unread chats and returns new messages.
func (l *Listener) pollOnce() []IncomingMessage {
	l.sender.mu.Lock()
	defer l.sender.mu.Unlock()

	if l.sender.page == nil || l.sender.page.IsClosed() {
		return nil
	}
	if !l.sender.checkLoggedInLocked() {
		return nil
	}

	chatItems, err := l.findUnreadChatItemsLocked()
	if err != nil {
		log.Printf("[whatsapp-listener] find unread chats: %v", err)
		return nil
	}

	var messages []IncomingMessage
	for _, item := range chatItems {
		msg, ok := l.extractMessageFromChatItemLocked(item)
		if !ok {
			continue
		}

		// Deduplicate: skip if we already saw a message from this phone at or after this time.
		if lastTime, exists := l.lastSeen[msg.Phone]; exists && !msg.Timestamp.After(lastTime) {
			continue
		}
		l.lastSeen[msg.Phone] = msg.Timestamp
		messages = append(messages, msg)
	}

	return messages
}

// findUnreadChatItemsLocked returns chat items that have unread indicators.
// Must be called with sender.mu held.
func (l *Listener) findUnreadChatItemsLocked() ([]string, error) {
	var chatListSelector string
	for _, sel := range ListenerSelectors["chat_list"] {
		loc := l.sender.page.Locator(sel)
		count, err := loc.Count()
		if err == nil && count > 0 {
			chatListSelector = sel
			break
		}
	}
	if chatListSelector == "" {
		return nil, fmt.Errorf("chat list not found")
	}

	chatList := l.sender.page.Locator(chatListSelector)

	var chatItemSelector string
	for _, sel := range ListenerSelectors["chat_item"] {
		chatItemSelector = sel
		break
	}

	items, err := chatList.Locator(chatItemSelector).All()
	if err != nil {
		return nil, fmt.Errorf("get chat items: %w", err)
	}

	var unreadItems []string
	for _, item := range items {
		hasUnread := false
		for _, badgeSel := range ListenerSelectors["unread_badge"] {
			badge := item.Locator(badgeSel)
			count, err := badge.Count()
			if err == nil && count > 0 {
				hasUnread = true
				break
			}
		}
		if hasUnread {
			unreadItems = append(unreadItems, chatItemSelector)
		}
	}

	// Fallback: if no unread badges found, return all items and let
	// extractMessageFromChatItemLocked filter by timestamp.
	// This handles WhatsApp UI changes where unread indicators differ.
	if len(unreadItems) == 0 && len(items) > 0 {
		for range items {
			unreadItems = append(unreadItems, chatItemSelector)
		}
	}

	return unreadItems, nil
}

// extractMessageFromChatItemLocked reads the contact name from a chat item.
// Must be called with sender.mu held.
func (l *Listener) extractMessageFromChatItemLocked(selector string) (IncomingMessage, bool) {
	var title string
	for _, titleSel := range ListenerSelectors["chat_item_title"] {
		loc := l.sender.page.Locator(titleSel)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		text, err := loc.First().InnerText()
		if err == nil && text != "" {
			title = strings.TrimSpace(text)
			break
		}
	}

	if title == "" {
		return IncomingMessage{}, false
	}

	now := time.Now()
	return IncomingMessage{
		Phone:     title,
		Name:      title,
		Text:      "",
		Timestamp: now,
		IsGroup:   strings.Contains(title, "…") || strings.ContainsAny(title, "()"),
		ChatTitle: title,
	}, true
}

// ReadRecentMessages opens a specific chat and reads recent incoming messages.
// This is more accurate but slower than sidebar polling.
func (l *Listener) ReadRecentMessages(ctx context.Context, phone string, limit int) ([]IncomingMessage, error) {
	l.sender.mu.Lock()
	defer l.sender.mu.Unlock()

	if l.sender.page == nil || l.sender.page.IsClosed() {
		return nil, fmt.Errorf("browser not running")
	}

	if err := l.navigateAndOpenChatLocked(phone); err != nil {
		return nil, err
	}

	return l.readMessagesFromOpenChatLocked(limit)
}

func (l *Listener) navigateAndOpenChatLocked(phone string) error {
	url := "https://web.whatsapp.com/send?phone=" + phone
	if _, err := l.sender.page.Goto(url); err != nil {
		return fmt.Errorf("navigate to chat: %w", err)
	}

	// Wait for messages to load.
	timeout := 5000.0
	for _, sel := range ListenerSelectors["message_area"] {
		if _, err := l.sender.page.WaitForSelector(sel, playwrightWaitForSelectorOptions(&timeout)); err == nil {
			return nil
		}
	}
	return fmt.Errorf("message area not found after navigation")
}

func (l *Listener) readMessagesFromOpenChatLocked(limit int) ([]IncomingMessage, error) {
	var areaSelector string
	for _, sel := range ListenerSelectors["message_area"] {
		loc := l.sender.page.Locator(sel)
		count, err := loc.Count()
		if err == nil && count > 0 {
			areaSelector = sel
			break
		}
	}
	if areaSelector == "" {
		return nil, fmt.Errorf("message area not found")
	}

	area := l.sender.page.Locator(areaSelector)
	var msgSelector string
	for _, sel := range ListenerSelectors["incoming_message"] {
		msgSelector = sel
		break
	}

	msgElements, err := area.Locator(msgSelector).All()
	if err != nil {
		return nil, fmt.Errorf("get message elements: %w", err)
	}

	var messages []IncomingMessage
	for i := len(msgElements) - 1; i >= 0 && len(messages) < limit; i-- {
		el := msgElements[i]
		text := l.extractTextFromMessageElementLocked(el)
		if text == "" {
			continue
		}
		messages = append(messages, IncomingMessage{
			Text:      text,
			Timestamp: time.Now(),
		})
	}

	return messages, nil
}

func (l *Listener) extractTextFromMessageElementLocked(el playwright.Locator) string {
	for _, textSel := range ListenerSelectors["message_text"] {
		loc := el.Locator(textSel)
		count, err := loc.Count()
		if err != nil || count == 0 {
			continue
		}
		text, err := loc.First().InnerText()
		if err == nil {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func playwrightWaitForSelectorOptions(timeout *float64) playwright.PageWaitForSelectorOptions {
	return playwright.PageWaitForSelectorOptions{Timeout: timeout}
}
