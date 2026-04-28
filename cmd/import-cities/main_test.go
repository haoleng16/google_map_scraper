package main

import "testing"

func TestParseCityRecord(t *testing.T) {
	line := "2643743\tLondon\tLondon\tLondres\t51.50853\t-0.12574\tP\tPPLC\tGB\t\tH9\t\t\t\t8961989\t\t25\tEurope/London\t2024-01-01"

	record, err := parseCityRecord(line)
	if err != nil {
		t.Fatal(err)
	}

	if record.ID != 2643743 {
		t.Fatalf("expected id 2643743, got %d", record.ID)
	}

	if record.Name != "London" || record.CountryCode != "GB" || record.Population != 8961989 {
		t.Fatalf("unexpected record: %+v", record)
	}

	if record.Latitude != 51.50853 || record.Longitude != -0.12574 {
		t.Fatalf("unexpected coordinates: %+v", record)
	}
}

func TestParseCityRecordRejectsShortLine(t *testing.T) {
	_, err := parseCityRecord("too\tshort")
	if err == nil {
		t.Fatal("expected error")
	}
}
