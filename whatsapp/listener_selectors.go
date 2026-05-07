package whatsapp

// ListenerSelectors maps UI element names to selectors used for reading incoming messages.
// Each key maps to a slice of fallback selectors tried in order.
var ListenerSelectors = map[string][]string{
	// The sidebar chat list container.
	"chat_list": {
		`div[aria-label="Chat list"]`,
		`div[aria-label="聊天列表"]`,
		`[data-testid="chat-list"]`,
		`#pane-side div[role="grid"]`,
	},
	// Individual chat items in the sidebar.
	"chat_item": {
		`div[role="row"]`,
		`div[role="listitem"]`,
		`[data-testid="cell-frame-container"]`,
	},
	// Unread badge showing new message count on a chat item.
	"unread_badge": {
		`span[data-testid="icon-unread-count"]`,
		`span[aria-label*="unread"]`,
		`span[aria-label*="未读"]`,
	},
	// The contact/group name in a chat item.
	"chat_item_title": {
		`span[dir="auto"][title]`,
		`span[title]`,
	},
	// The main message area when a chat is open.
	"message_area": {
		`div[data-testid="conversation-panel-messages"]`,
		`div[role="application"]`,
	},
	// Individual incoming message bubbles.
	"incoming_message": {
		`div.message-in`,
		`div[data-testid="msg-container"]`,
	},
	// The text content of a message bubble.
	"message_text": {
		`span.selectable-text span[dir="ltr"]`,
		`span.selectable-text span[dir="auto"]`,
		`span.selectable-text`,
	},
	// Search input in the sidebar.
	"search_input": {
		`div[contenteditable="true"][data-tab="3"]`,
		`div[contenteditable="true"][title="搜索或开始新对话"]`,
		`div[contenteditable="true"][title="Search input textbox"]`,
	},
}
