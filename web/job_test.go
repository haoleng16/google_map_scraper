package web

import "testing"

func TestJobDataSetDefaultLang(t *testing.T) {
	tests := []struct {
		name     string
		data     JobData
		expected string
	}{
		{
			name:     "defaults english for latin keywords",
			data:     JobData{Keywords: []string{"mobile phone shop"}},
			expected: "en",
		},
		{
			name:     "uses chinese for han keywords",
			data:     JobData{Keywords: []string{"手机店"}},
			expected: "zh",
		},
		{
			name:     "preserves explicit language",
			data:     JobData{Lang: "de", Keywords: []string{"手机店"}},
			expected: "de",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.data.SetDefaultLang()

			if tt.data.Lang != tt.expected {
				t.Fatalf("expected lang %q, got %q", tt.expected, tt.data.Lang)
			}
		})
	}
}
