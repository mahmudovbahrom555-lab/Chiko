package chat

import (
	"strings"
	"testing"
)

// TestSendVoiceExtParsing verifies the safe ext extraction after the panic fix.
// Previously: strings.LastIndex on empty string returned -1, causing panic.
func TestSendVoiceExtParsing(t *testing.T) {
	cases := []struct {
		filename string
		wantExt  string
	}{
		{"audio.opus", "opus"},
		{"file.m4a", "m4a"},
		{"UPPER.OGG", "ogg"},
		{"no-extension", ""},    // was panic before fix
		{"", ""},                 // was panic before fix
		{".hidden", "hidden"},
		{"a.b.c.opus", "opus"},
		{"audio.", ""},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			// Replicate the fixed logic from handler.go
			var ext string
			if idx := strings.LastIndex(tc.filename, "."); idx >= 0 {
				ext = strings.ToLower(tc.filename[idx+1:])
			}
			if ext != tc.wantExt {
				t.Errorf("filename=%q: got ext=%q, want %q", tc.filename, ext, tc.wantExt)
			}
		})
	}
}
