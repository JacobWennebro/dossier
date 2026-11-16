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

// ansiToHTML converts text with SGR escape sequences (reset, bold, 16 and
// 256 colors) into HTML-escaped text wrapped in styled <span>s. Other escape
// sequences are dropped.
func ansiToHTML(s string) string {
	var (
		b    strings.Builder
		fg   string
		bold bool
		open bool
	)
	setStyle := func(newFG string, newBold bool) {
		if newFG == fg && newBold == bold {
			return
		}
		if open {
			b.WriteString("</span>")
			open = false
		}
		fg, bold = newFG, newBold
		if fg == "" && !bold {
			return
		}
		b.WriteString(`<span style="`)
		if fg != "" {
			b.WriteString("color:" + fg + ";")
		}
		if bold {
			b.WriteString("font-weight:bold;")
		}
		b.WriteString(`">`)
		open = true
	}

	for len(s) > 0 {
		i := strings.IndexByte(s, 0x1b)
		if i < 0 {
			b.WriteString(html.EscapeString(s))
			break
		}
		b.WriteString(html.EscapeString(s[:i]))
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

		newFG, newBold := fg, bold
		codes := strings.Split(params, ";")
		for j := 0; j < len(codes); j++ {
			n, _ := strconv.Atoi(codes[j]) // "" means 0
			switch {
			case n == 0:
				newFG, newBold = "", false
			case n == 1:
				newBold = true
			case n == 22:
				newBold = false
			case n >= 30 && n <= 37:
				newFG = basic16[n-30]
			case n >= 90 && n <= 97:
				newFG = basic16[n-90+8]
			case n == 39:
				newFG = ""
			case n == 38 && j+2 < len(codes) && codes[j+1] == "5":
				if c, err := strconv.Atoi(codes[j+2]); err == nil && c >= 0 && c < 256 {
					newFG = color256(c)
				}
				j += 2
			case n == 38 && j+4 < len(codes) && codes[j+1] == "2":
				r, _ := strconv.Atoi(codes[j+2])
				g, _ := strconv.Atoi(codes[j+3])
				bl, _ := strconv.Atoi(codes[j+4])
				newFG = fmt.Sprintf("#%02x%02x%02x", r&255, g&255, bl&255)
				j += 4
			}
		}
		setStyle(newFG, newBold)
	}
	if open {
		b.WriteString("</span>")
	}
	return b.String()
}
