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

func TestParseCountryRecord(t *testing.T) {
	line := "VN\tVNM\t704\tVM\tVietnam\tHanoi\t329560\t97338579\tAS\t.vn\tVND\tDong\t84\t#####\t^(\\d{5})$\tvi,en,fr,zh,km\t1562822\tCN,LA,KH\t"

	record, err := parseCountryRecord(line)
	if err != nil {
		t.Fatal(err)
	}

	if record.Code != "VN" || record.Name != "Vietnam" || record.Capital != "Hanoi" {
		t.Fatalf("unexpected country: %+v", record)
	}

	if record.AreaSqKm != 329560 || record.Population != 97338579 {
		t.Fatalf("unexpected numeric fields: %+v", record)
	}
}

func TestCountryAliasesIncludesTranslationsAndManualAliases(t *testing.T) {
	record := countryRecord{Code: "VN", Name: "Vietnam"}
	got := countryAliases(record, []string{"越南", "Viet Nam"})

	for _, want := range []string{"VN", "Vietnam", "越南", "Viet Nam", "Việt Nam"} {
		if !contains(got, want) {
			t.Fatalf("expected alias %q in %#v", want, got)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
