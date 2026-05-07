package whatsapp

import (
	"encoding/csv"
	"io"
	"strings"
)

// ParseContacts reads CSV data and returns parsed WhatsApp contacts.
func ParseContacts(reader io.Reader) ([]Contact, error) {
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
		headerIndex[normalizeCSVHeader(header)] = i
	}

	contacts := make([]Contact, 0, len(rows)-1)
	for _, row := range rows[1:] {
		phone := CleanPhone(csvValue(row, headerIndex, "phone", "电话"))
		if phone == "" {
			continue
		}

		contacts = append(contacts, Contact{
			ID:       phone,
			ShopName: csvValue(row, headerIndex, "title", "name", "商家名称", "名称"),
			Phone:    phone,
			Address:  csvValue(row, headerIndex, "address", "地址"),
			Category: csvValue(row, headerIndex, "category", "分类"),
			Rating:   csvValue(row, headerIndex, "review_rating", "rating", "评分"),
			Email:    csvValue(row, headerIndex, "emails", "email", "邮箱"),
			Website:  csvValue(row, headerIndex, "web_site", "website", "官网", "网站"),
			Selected: true,
		})
	}

	return contacts, nil
}

// CleanPhone removes formatting characters from a phone number string.
func CleanPhone(phone string) string {
	return PhoneCleaner.ReplaceAllString(strings.TrimSpace(phone), "")
}

// csvValue returns the value from a CSV row matching one of the given header names.
func csvValue(row []string, headerIndex map[string]int, names ...string) string {
	for _, name := range names {
		if idx, ok := headerIndex[normalizeCSVHeader(name)]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}
	return ""
}

func normalizeCSVHeader(header string) string {
	return strings.ToLower(strings.TrimSpace(header))
}
