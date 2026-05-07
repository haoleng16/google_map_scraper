package whatsapp

import "testing"

func TestSendOptionsNormalizeUsesCompliantDefaults(t *testing.T) {
	options := (SendOptions{}).Normalize()

	if options.ContactDelayMinSeconds != 1 || options.ContactDelayMaxSeconds != 5 {
		t.Fatalf("contact delay = %d-%d, want 1-5", options.ContactDelayMinSeconds, options.ContactDelayMaxSeconds)
	}
	if options.BatchSize != 20 {
		t.Fatalf("batch size = %d, want 20", options.BatchSize)
	}
	if options.BatchDelayMinSeconds != 5 || options.BatchDelayMaxSeconds != 10 {
		t.Fatalf("batch delay = %d-%d, want 5-10", options.BatchDelayMinSeconds, options.BatchDelayMaxSeconds)
	}
	if options.MaxConsecutiveFailures != 5 {
		t.Fatalf("max consecutive failures = %d, want 5", options.MaxConsecutiveFailures)
	}
}

func TestSendOptionsNormalizeSwapsInvalidRanges(t *testing.T) {
	options := SendOptions{
		ContactDelayMinSeconds: 9,
		ContactDelayMaxSeconds: 2,
		BatchSize:              999,
		BatchDelayMinSeconds:   15,
		BatchDelayMaxSeconds:   7,
		MaxConsecutiveFailures: 200,
	}.Normalize()

	if options.ContactDelayMinSeconds != 2 || options.ContactDelayMaxSeconds != 9 {
		t.Fatalf("contact delay = %d-%d, want swapped 2-9", options.ContactDelayMinSeconds, options.ContactDelayMaxSeconds)
	}
	if options.BatchSize != 500 {
		t.Fatalf("batch size = %d, want clamp to 500", options.BatchSize)
	}
	if options.BatchDelayMinSeconds != 7 || options.BatchDelayMaxSeconds != 15 {
		t.Fatalf("batch delay = %d-%d, want swapped 7-15", options.BatchDelayMinSeconds, options.BatchDelayMaxSeconds)
	}
	if options.MaxConsecutiveFailures != 100 {
		t.Fatalf("max consecutive failures = %d, want clamp to 100", options.MaxConsecutiveFailures)
	}
}
