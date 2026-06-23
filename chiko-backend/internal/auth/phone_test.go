package auth_test

import (
	"testing"

	"github.com/chiko/backend/internal/auth"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		raw     string
		region  string
		want    string
		wantErr bool
	}{
		// Узбекистан (+998)
		{"+998901234567", "UZ", "+998901234567", false},
		{"998901234567", "UZ", "+998901234567", false},
		{"901234567", "UZ", "+998901234567", false},

		// США (+1) — проверяем что не хардкодим одну страну
		{"+12025550123", "US", "+12025550123", false},
		{"2025550123", "US", "+12025550123", false},

		// ОАЭ (+971)
		{"+971501234567", "AE", "+971501234567", false},

		// Казахстан (+7)
		{"+77001234567", "KZ", "+77001234567", false},

		// Турция (+90)
		{"+905551234567", "TR", "+905551234567", false},

		// Невалидный номер
		{"123", "UZ", "", true},
		{"notanumber", "", "", true},
		{"", "UZ", "", true},
	}

	for _, tc := range cases {
		got, err := auth.NormalizePhone(tc.raw, tc.region)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizePhone(%q, %q): expected error, got nil", tc.raw, tc.region)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizePhone(%q, %q): unexpected error: %v", tc.raw, tc.region, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizePhone(%q, %q) = %q, want %q", tc.raw, tc.region, got, tc.want)
		}
	}
}

// TestNormalizePhone_E164Format проверяет что результат всегда начинается с '+'
func TestNormalizePhone_E164Format(t *testing.T) {
	numbers := []struct{ raw, region string }{
		{"+998901234567", "UZ"},
		{"+12025550123", "US"},
		{"+971501234567", "AE"},
		{"+77001234567", "KZ"},
	}
	for _, tc := range numbers {
		got, err := auth.NormalizePhone(tc.raw, tc.region)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tc.raw, err)
		}
		if len(got) == 0 || got[0] != '+' {
			t.Errorf("NormalizePhone(%q) = %q: not E.164 format (must start with '+')", tc.raw, got)
		}
	}
}
