package main

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// basic16 is the palette for SGR 30–37 and 90–97 (and 256-color 0–15),
// tuned for a dark background.
var basic16 = [16]string{
	"#484f58", "#ff7b72", "#3fb950", "#d29922", "#58a6ff", "#bc8cff", "#39c5cf", "#e6edf3",
	"#6e7681", "#ffa198", "#56d364", "#e3b341", "#79c0ff", "#d2a8ff", "#56d4dd", "#ffffff",
}

// color256 returns the hex color of an xterm 256-color index.
func color256(n int) string {
	switch {
	case n < 16:
		return basic16[n]
	case n < 232:
		n -= 16
		level := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return fmt.Sprintf("#%02x%02x%02x", level(n/36), level(n/6%6), level(n%6))
	default:
		g := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", g, g, g)
	}
}

// segment is a run of text drawn in one style. FG is a hex color, or ""
// for the default foreground.
type segment struct {
	Text string
	FG   string
	Bold bool
}

// parseANSI splits text with SGR escape sequences (reset, bold, 16, 256 and
// 24-bit colors) into styled segments. Other escape sequences are dropped.
// Adjacent segments always differ in style.
func parseANSI(s string) []segment {
	var (
		segs []segment
		fg   string
		bold bool
	)
	emit := func(text string) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].FG == fg && segs[n-1].Bold == bold {
			segs[n-1].Text += text
			return
		}
		segs = append(segs, segment{text, fg, bold})
	}

	for len(s) > 0 {
		i := strings.IndexByte(s, 0x1b)
		if i < 0 {
			emit(s)
			break
		}
		emit(s[:i])
		s = s[i+1:]
		if len(s) == 0 || s[0] != '[' {
			continue
		}
		// CSI: parameters and intermediates, then a final byte in 0x40–0x7e.
		end := strings.IndexFunc(s[1:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
		if end < 0 {
			break
		}
		params, final := s[1:1+end], s[1+end]
		s = s[2+end:]
		if final != 'm' {
			continue
		}

		codes := strings.Split(params, ";")
		for j := 0; j < len(codes); j++ {
			n, _ := strconv.Atoi(codes[j]) // "" means 0
			switch {
			case n == 0:
				fg, bold = "", false
			case n == 1:
				bold = true
			case n == 22:
				bold = false
			case n >= 30 && n <= 37:
				fg = basic16[n-30]
			case n >= 90 && n <= 97:
				fg = basic16[n-90+8]
			case n == 39:
				fg = ""
			case n == 38 && j+2 < len(codes) && codes[j+1] == "5":
				if c, err := strconv.Atoi(codes[j+2]); err == nil && c >= 0 && c < 256 {
					fg = color256(c)
				}
				j += 2
			case n == 38 && j+4 < len(codes) && codes[j+1] == "2":
				r, _ := strconv.Atoi(codes[j+2])
				g, _ := strconv.Atoi(codes[j+3])
				bl, _ := strconv.Atoi(codes[j+4])
				fg = fmt.Sprintf("#%02x%02x%02x", r&255, g&255, bl&255)
				j += 4
			}
		}
	}
	return segs
}

// ansiToHTML converts text with SGR escape sequences into HTML-escaped text
// wrapped in styled <span>s.
func ansiToHTML(s string) string {
	var b strings.Builder
	for _, seg := range parseANSI(s) {
		text := html.EscapeString(seg.Text)
		if seg.FG == "" && !seg.Bold {
			b.WriteString(text)
			continue
		}
		b.WriteString(`<span style="`)
		if seg.FG != "" {
			b.WriteString("color:" + seg.FG + ";")
		}
		if seg.Bold {
			b.WriteString("font-weight:bold;")
		}
		b.WriteString(`">` + text + "</span>")
	}
	return b.String()
}
