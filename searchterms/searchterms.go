package searchterms

import (
	"strings"
	"unicode"
)

var zhBusinessTerms = map[string][]string{
	"手机店":  {"mobile phone shop", "cell phone store"},
	"手机维修": {"phone repair shop", "mobile phone repair"},
	"餐厅":   {"restaurant"},
	"饭店":   {"restaurant"},
	"酒店":   {"hotel"},
	"咖啡店":  {"coffee shop", "cafe"},
	"牙科":   {"dentist"},
	"牙医":   {"dentist"},
	"医院":   {"hospital"},
	"药店":   {"pharmacy"},
	"超市":   {"supermarket"},
	"便利店":  {"convenience store"},
	"理发店":  {"barber shop", "hair salon"},
	"美容院":  {"beauty salon"},
	"健身房":  {"gym", "fitness center"},
	"学校":   {"school"},
	"律师":   {"lawyer"},
	"会计":   {"accountant"},
	"宠物店":  {"pet store"},
	"汽修":   {"auto repair shop", "car repair"},
	"汽车维修": {"auto repair shop", "car repair"},
	"房产中介": {"real estate agency"},
}

var zhBusinessTermOrder = []string{
	"手机维修",
	"手机店",
	"餐厅",
	"饭店",
	"酒店",
	"咖啡店",
	"牙科",
	"牙医",
	"医院",
	"药店",
	"超市",
	"便利店",
	"理发店",
	"美容院",
	"健身房",
	"学校",
	"律师",
	"会计",
	"宠物店",
	"汽车维修",
	"汽修",
	"房产中介",
}

func Expand(keywords []string) []string {
	seen := make(map[string]struct{})
	var expanded []string

	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}

		addUnique(&expanded, seen, keyword)

		if hasHan(keyword) {
			for _, translated := range translateChineseKeyword(keyword) {
				addUnique(&expanded, seen, translated)
			}
		}
	}

	return expanded
}

func ExpandForLocation(keywords []string, location string) []string {
	if !IsForeignLocation(location) {
		return Expand(keywords)
	}

	seen := make(map[string]struct{})
	var expanded []string

	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}

		translated := translateChineseKeyword(keyword)
		if hasHan(keyword) && len(translated) > 0 {
			for _, term := range translated {
				addUnique(&expanded, seen, term)
			}

			continue
		}

		addUnique(&expanded, seen, keyword)
		for _, term := range translated {
			addUnique(&expanded, seen, term)
		}
	}

	return expanded
}

func translateChineseKeyword(keyword string) []string {
	if translated, ok := zhBusinessTerms[keyword]; ok {
		return translated
	}

	seen := make(map[string]struct{})
	var translations []string
	for _, zh := range zhBusinessTermOrder {
		if strings.Contains(keyword, zh) {
			for _, translated := range zhBusinessTerms[zh] {
				addUnique(&translations, seen, translated)
			}
		}
	}

	return translations
}

func addUnique(values *[]string, seen map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}

	key := strings.ToLower(value)
	if _, ok := seen[key]; ok {
		return
	}

	seen[key] = struct{}{}
	*values = append(*values, value)
}

func hasHan(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}

	return false
}

func IsForeignLocation(location string) bool {
	location = strings.ToLower(strings.TrimSpace(location))
	if location == "" {
		return false
	}

	return !strings.Contains(location, "中国") && !strings.Contains(location, "china")
}
