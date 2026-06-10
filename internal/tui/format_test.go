package tui

import (
	"strings"
	"testing"
)

func TestSanitizeTextStripsEscapesAndControls(t *testing.T) {
	cases := map[string]string{
		"plain":                  "plain",
		"\x1b[31mred\x1b[0m":     "[31mred[0m", // ESC removed, payload inert
		"a\x00b\x07c":            "abc",
		"tab\tnewline\r\n":       "tabnewline",
		"emoji 🌾 stays":          "emoji 🌾 stays",
		"\x1b]0;evil title\x07x": "]0;evil titlex",
	}
	for in, want := range cases {
		if got := sanitizeText(in); got != want {
			t.Errorf("sanitizeText(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.ContainsRune(sanitizeText("\x1b\x1b\x1b"), '\x1b') {
		t.Fatal("escape characters survived sanitization")
	}
}

func TestMoney(t *testing.T) {
	cases := map[int64]string{
		0: "0", 7: "7", 999: "999", 1000: "1,000",
		1234567: "1,234,567", -4500: "-4,500",
	}
	for in, want := range cases {
		if got := money(in); got != want {
			t.Errorf("money(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	cases := map[int64]string{
		0: "0s", 59: "59s", 94: "1m 34s", 3600: "1h 00m",
		90061: "1d 1h", -5: "0s",
	}
	for in, want := range cases {
		if got := duration(in); got != want {
			t.Errorf("duration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestProgressBarBounds(t *testing.T) {
	if got := progressBar(-10, 4); got != "▱▱▱▱" {
		t.Errorf("negative pct: %q", got)
	}
	if got := progressBar(250, 4); got != "▰▰▰▰" {
		t.Errorf("over-100 pct: %q", got)
	}
	if got := progressBar(50, 4); got != "▰▰▱▱" {
		t.Errorf("half: %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 3); got != "he…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("no-op truncate = %q", got)
	}
	if got := truncate("héllo wörld", 5); got != "héll…" {
		t.Errorf("unicode truncate = %q", got)
	}
}
