package identity

import "testing"

func TestSanitizeSlot(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Nigel", "nigel"},
		{"other-name_2", "other-name_2"},
		{"bad/name", "badname"},
		{"", ""},
		{"!!!", ""},
		{"a" + string(make([]byte, 40)), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, tc := range tests {
		if tc.in != "" && len(tc.in) > 40 {
			tc.in = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}
		got := SanitizeSlot(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeSlot(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveSlotUsesDefault(t *testing.T) {
	if got := ResolveSlot("", "default"); got != "default" {
		t.Fatalf("ResolveSlot empty = %q", got)
	}
	if got := ResolveSlot("!!!", "default"); got != "default" {
		t.Fatalf("ResolveSlot invalid = %q", got)
	}
}
