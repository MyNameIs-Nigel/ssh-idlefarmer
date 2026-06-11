package server

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestCursorDownWriterRewritesNewlines checks the byte-level transform: every
// \n becomes ESC[B and all other bytes pass through unchanged, including when
// the input is split across multiple Write calls (the writer must be
// stateless, since a real renderer flushes in arbitrary chunks).
func TestCursorDownWriterRewritesNewlines(t *testing.T) {
	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"no newline", []string{"\x1b[5;1Hhello"}, "\x1b[5;1Hhello"},
		{"single", []string{"a\nb"}, "a\x1b[Bb"},
		{"crlf preserved as cr+down", []string{"a\r\nb"}, "a\r\x1b[Bb"},
		{"run of newlines", []string{"x\n\n\ny"}, "x\x1b[B\x1b[B\x1b[By"},
		{"split across writes", []string{"a", "\n", "b\n", "c"}, "a\x1b[Bb\x1b[Bc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sink bytes.Buffer
			w := &cursorDownWriter{w: &sink}
			total := 0
			for _, c := range tc.chunks {
				n, err := w.Write([]byte(c))
				if err != nil {
					t.Fatalf("Write(%q) error: %v", c, err)
				}
				// Write must report the caller's byte count, not the expanded one.
				if n != len(c) {
					t.Errorf("Write(%q) returned n=%d, want %d", c, n, len(c))
				}
				total += n
			}
			if got := sink.String(); got != tc.want {
				t.Errorf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCursorDownWriterPreventsLeftBleed is the regression test for the Windows
// alt-screen smear. It replays the exact byte pattern Bubble Tea's diff
// renderer emits for a uniform-style multi-line block — position the first
// line absolutely, then move down and write the next line directly, with no
// horizontal reposition (because the renderer's mapNl=false model believes the
// column is preserved). On a Windows console, where \n acts as CR+LF, the raw
// stream collapses the trailing lines to column 0. Routing the same stream
// through cursorDownWriter (which turns \n into a column-preserving cursor
// down) keeps every line at its intended column.
func TestCursorDownWriterPreventsLeftBleed(t *testing.T) {
	const startCol = 50 // 0-based column of the framed body, 1-based 51

	// Render a uniform block: CUP to (row=6, col=51), then two more rows reached
	// by a bare \n and written without any horizontal move.
	raw := "\x1b[2J" +
		"\x1b[6;51H" + "Plots owned: 1" +
		"\n" + "Your harvesting" +
		"\n" + "Each plot costs"

	rawCols := replayLeftmostCols(t, raw)

	var wrapped bytes.Buffer
	w := &cursorDownWriter{w: &wrapped}
	if _, err := w.Write([]byte(raw)); err != nil {
		t.Fatalf("writer error: %v", err)
	}
	fixedCols := replayLeftmostCols(t, wrapped.String())

	// Sanity: the raw stream must actually reproduce the bug, otherwise this
	// test proves nothing.
	bled := false
	for _, c := range rawCols {
		if c == 0 {
			bled = true
		}
	}
	if !bled {
		t.Fatal("raw stream did not reproduce the col-0 bleed; test is not exercising the bug")
	}

	// The fix: no rendered line may start in the dead left margin.
	for row, c := range fixedCols {
		if c < startCol {
			t.Errorf("row %d rendered at col %d, inside the left margin (< %d) — bleed not fixed", row, c, startCol)
		}
	}
}

var (
	cupRe = regexp.MustCompile(`^\x1b\[(\d+);(\d+)H`)
	csiRe = regexp.MustCompile(`^\x1b\[([0-9;]*)([A-Za-z])`)
)

// replayLeftmostCols is a tiny VT replay that emulates a Windows console (\n =
// CR+LF) and returns, per row that has any printable content, the 0-based
// column of its first printed character. Only the handful of sequences the
// renderer emits here are interpreted; SGR and the rest are skipped.
func replayLeftmostCols(t *testing.T, s string) map[int]int {
	t.Helper()
	const w, h = 200, 50
	first := map[int]int{} // row -> leftmost printed col
	x, y := 0, 0
	clampX := func(v int) int {
		if v < 0 {
			return 0
		}
		if v >= w {
			return w - 1
		}
		return v
	}
	clampY := func(v int) int {
		if v < 0 {
			return 0
		}
		if v >= h {
			return h - 1
		}
		return v
	}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\x1b' {
			if m := cupRe.FindStringSubmatch(s[i:]); m != nil {
				y = clampY(atoi(m[1]) - 1)
				x = clampX(atoi(m[2]) - 1)
				i += len(m[0])
				continue
			}
			if m := csiRe.FindStringSubmatch(s[i:]); m != nil {
				n := atoi(m[1])
				if n == 0 {
					n = 1
				}
				switch m[2] {
				case "A":
					y = clampY(y - n)
				case "B":
					y = clampY(y + n) // cursor down keeps the column
				case "C":
					x = clampX(x + n)
				case "D":
					x = clampX(x - n)
				case "G":
					x = clampX(atoi(m[1]) - 1)
				case "d":
					y = clampY(atoi(m[1]) - 1)
				}
				i += len(m[0])
				continue
			}
			i++
			continue
		}
		switch c {
		case '\r':
			x = 0
		case '\n':
			y = clampY(y + 1)
			x = 0 // Windows console: \n implies carriage return
		default:
			if c >= 0x20 {
				if _, seen := first[y]; !seen {
					first[y] = x
				}
				x = clampX(x + 1)
			}
		}
		i++
	}
	return first
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
