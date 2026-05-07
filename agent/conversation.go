package agent

import "sync"

// ConversationStore manages conversation history per phone number.
// It keeps a sliding window of recent messages for LLM context.
type ConversationStore struct {
	mu     sync.Mutex
	window int
	store  map[string][]ChatMessage
}

// NewConversationStore creates a store with the given context window size.
func NewConversationStore(windowSize int) *ConversationStore {
	if windowSize <= 0 {
		windowSize = 20
	}
	return &ConversationStore{
		window: windowSize,
		store:  make(map[string][]ChatMessage),
	}
}

// Add appends a message to the conversation for the given phone.
func (cs *ConversationStore) Add(phone string, msg ChatMessage) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	msgs := cs.store[phone]
	msgs = append(msgs, msg)

	// Trim to window size, keeping the most recent messages.
	if len(msgs) > cs.window {
		msgs = msgs[len(msgs)-cs.window:]
	}
	cs.store[phone] = msgs
}

// Get returns the recent message history for a phone number.
func (cs *ConversationStore) Get(phone string) []ChatMessage {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	msgs := cs.store[phone]
	out := make([]ChatMessage, len(msgs))
	copy(out, msgs)
	return out
}

// Clear removes conversation history for a phone number.
func (cs *ConversationStore) Clear(phone string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.store, phone)
}

// Reset reloads conversation history from a list of messages (e.g. from DB).
func (cs *ConversationStore) Reset(phone string, msgs []ChatMessage) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(msgs) > cs.window {
		msgs = msgs[len(msgs)-cs.window:]
	}
	cs.store[phone] = msgs
}
