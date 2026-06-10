package tui

import (
	"fmt"
	"strings"
	"unicode"
)

// sanitizeText strips control characters and escape sequences from any
// user- or content-influenced string before it reaches the terminal. Slots
// are already sanitized at the door (Task 1) and content files are operator
// config, but output escaping is defense in depth: nothing dynamic may carry
// a terminal escape.
func sanitizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\x1b' || unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// money renders coins with thousands separators: 1234567 -> "1,234,567".
func money(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) > 3 {
		var b strings.Builder
		lead := len(s) % 3
		if lead > 0 {
			b.WriteString(s[:lead])
		}
		for i := lead; i < len(s); i += 3 {
			if b.Len() > 0 {
				b.WriteByte(',')
			}
			b.WriteString(s[i : i+3])
		}
		s = b.String()
	}
	if neg {
		return "-" + s
	}
	return s
}

// duration humanizes a second count: 94 -> "1m 34s", 90061 -> "1d 1h".
func duration(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm %02ds", secs/60, secs%60)
	case secs < 86400:
		return fmt.Sprintf("%dh %02dm", secs/3600, (secs%3600)/60)
	default:
		return fmt.Sprintf("%dd %dh", secs/86400, (secs%86400)/3600)
	}
}

// progressBar renders a width-character bar at pct (0-100).
func progressBar(pct, width int) string {
	if width < 1 {
		width = 1
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	return strings.Repeat("▰", filled) + strings.Repeat("▱", width-filled)
}

// shortFingerprint trims "SHA256:..." for display.
func shortFingerprint(fp string) string {
	const max = 19 // "SHA256:" + 12 chars
	if len(fp) <= max {
		return fp
	}
	return fp[:max] + "…"
}

// truncate cuts s to max runes, appending "…" when cut.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
