package whatsapp

import (
	"regexp"
	"time"
)

const (
	MaxImageSize = 10 * 1024 * 1024
	MaxPDFSize   = 20 * 1024 * 1024
)

// PhoneCleaner strips formatting characters from phone numbers.
var PhoneCleaner = regexp.MustCompile(`[+\-\s()]`)

// Contact represents a WhatsApp contact parsed from CSV data.
type Contact struct {
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

// DisplayName returns the shop name if present, otherwise the phone number.
func (c Contact) DisplayName() string {
	if c.ShopName != "" {
		return c.ShopName
	}
	return c.Phone
}

// File represents an uploaded file (image or PDF) ready for sending.
type File struct {
	Path string `json:"-"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// Message represents a WhatsApp message with optional image and PDF attachments.
type Message struct {
	Text      string `json:"text"`
	ImageID   string `json:"image_id"`
	PDFID     string `json:"pdf_id"`
	ImagePath string `json:"-"`
	PDFPath   string `json:"-"`
}

// SendOptions controls compliant send pacing and automatic safety pauses.
type SendOptions struct {
	ContactDelayMinSeconds int `json:"contact_delay_min_seconds"`
	ContactDelayMaxSeconds int `json:"contact_delay_max_seconds"`
	BatchSize              int `json:"batch_size"`
	BatchDelayMinSeconds   int `json:"batch_delay_min_seconds"`
	BatchDelayMaxSeconds   int `json:"batch_delay_max_seconds"`
	MaxConsecutiveFailures int `json:"max_consecutive_failures"`
}

// DefaultSendOptions returns conservative pacing defaults for WhatsApp sends.
func DefaultSendOptions() SendOptions {
	return SendOptions{
		ContactDelayMinSeconds: 1,
		ContactDelayMaxSeconds: 5,
		BatchSize:              20,
		BatchDelayMinSeconds:   5,
		BatchDelayMaxSeconds:   10,
		MaxConsecutiveFailures: 5,
	}
}

// Normalize clamps user-provided pacing values to safe, usable ranges.
func (o SendOptions) Normalize() SendOptions {
	defaults := DefaultSendOptions()
	if o.ContactDelayMinSeconds <= 0 {
		o.ContactDelayMinSeconds = defaults.ContactDelayMinSeconds
	}
	if o.ContactDelayMaxSeconds <= 0 {
		o.ContactDelayMaxSeconds = defaults.ContactDelayMaxSeconds
	}
	if o.ContactDelayMinSeconds > o.ContactDelayMaxSeconds {
		o.ContactDelayMinSeconds, o.ContactDelayMaxSeconds = o.ContactDelayMaxSeconds, o.ContactDelayMinSeconds
	}
	if o.ContactDelayMaxSeconds > 300 {
		o.ContactDelayMaxSeconds = 300
	}
	if o.ContactDelayMinSeconds > o.ContactDelayMaxSeconds {
		o.ContactDelayMinSeconds = o.ContactDelayMaxSeconds
	}

	if o.BatchSize <= 0 {
		o.BatchSize = defaults.BatchSize
	}
	if o.BatchSize > 500 {
		o.BatchSize = 500
	}
	if o.BatchDelayMinSeconds <= 0 {
		o.BatchDelayMinSeconds = defaults.BatchDelayMinSeconds
	}
	if o.BatchDelayMaxSeconds <= 0 {
		o.BatchDelayMaxSeconds = defaults.BatchDelayMaxSeconds
	}
	if o.BatchDelayMinSeconds > o.BatchDelayMaxSeconds {
		o.BatchDelayMinSeconds, o.BatchDelayMaxSeconds = o.BatchDelayMaxSeconds, o.BatchDelayMinSeconds
	}
	if o.BatchDelayMaxSeconds > 3600 {
		o.BatchDelayMaxSeconds = 3600
	}
	if o.BatchDelayMinSeconds > o.BatchDelayMaxSeconds {
		o.BatchDelayMinSeconds = o.BatchDelayMaxSeconds
	}

	if o.MaxConsecutiveFailures <= 0 {
		o.MaxConsecutiveFailures = defaults.MaxConsecutiveFailures
	}
	if o.MaxConsecutiveFailures > 100 {
		o.MaxConsecutiveFailures = 100
	}

	return o
}

// IncomingMessage represents a message received from a WhatsApp chat.
type IncomingMessage struct {
	Phone     string    `json:"phone"`
	Name      string    `json:"name"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	IsGroup   bool      `json:"is_group"`
	ChatTitle string    `json:"chat_title"`
}

// HTTPError represents a structured error with status code and error code.
type HTTPError struct {
	Status  int
	Message string
	Code    string
}

func (e HTTPError) Error() string {
	return e.Message
}
