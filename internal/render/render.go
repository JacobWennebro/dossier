// Package render draws fastfetch-style output: an ASCII logo on the left and
// a title with key/value rows on the right.
package render

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	gap   = 3
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
)

// Row is one entry in the info column. Extra values continue on the
// following lines, aligned under the first value.
type Row struct {
	Key    string
	Values []string
	Bad    bool // draw the values in red, e.g. for missing records
}

// Card is everything needed to draw the output.
type Card struct {
	Title string
	Rows  []Row
	Logo  string // key in Logos; "none" hides the logo; unknown falls back to "generic"
}

// LogoNames returns the available logo names, sorted.
func LogoNames() []string {
	return slices.Sorted(maps.Keys(Logos))
}

// Render writes the card to w, with ANSI colors when color is true.
func Render(w io.Writer, c Card, color bool) error {
	var logo *Logo
	if c.Logo != "none" {
		l, ok := Logos[c.Logo]
		if !ok {
			l = Logos["generic"]
		}
		logo = &l
	}

	keyColor := "1"
	if logo != nil && len(logo.Colors) > 0 {
		keyColor = "1;" + logo.Colors[0]
	}
	paint := func(sgr, s string) string {
		if !color {
			return s
		}
		return "\x1b[" + sgr + "m" + s + reset
	}

	info := []string{
		paint(keyColor, c.Title),
		strings.Repeat("-", utf8.RuneCountInString(c.Title)),
	}
	for _, r := range c.Rows {
		indent := strings.Repeat(" ", utf8.RuneCountInString(r.Key)+2)
		for i, v := range r.Values {
			if r.Bad {
				v = paint("31", v)
			}
			if i == 0 {
				info = append(info, paint(keyColor, r.Key)+": "+v)
			} else {
				info = append(info, indent+v)
			}
		}
	}

	var left []string
	width := 0
	if logo != nil {
		for _, line := range logo.Lines {
			width = max(width, visibleWidth(line))
		}
		left = renderLogo(*logo, color)
		width += gap
	}

	for i := range max(len(left), len(info)) {
		var line string
		if i < len(left) {
			line = left[i]
		}
		if i < len(info) {
			pad := width - visibleWidth(logoLine(logo, i))
			line += strings.Repeat(" ", pad) + info[i]
		}
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " ")); err != nil {
			return err
		}
	}
	return nil
}

func logoLine(l *Logo, i int) string {
	if l == nil || i >= len(l.Lines) {
		return ""
	}
	return l.Lines[i]
}

// renderLogo expands $N markers into colors (or strips them). The active
// color carries across lines and is reset at the end of each line so the
// info column starts uncolored.
func renderLogo(l Logo, color bool) []string {
	out := make([]string, len(l.Lines))
	current := ""
	for i, line := range l.Lines {
		var b strings.Builder
		if _, starts := marker(line, 0); color && current != "" && !starts {
			b.WriteString("\x1b[" + current + "m")
		}
		for j := 0; j < len(line); j++ {
			if n, ok := marker(line, j); ok {
				if n < len(l.Colors) {
					current = l.Colors[n]
					if color {
						b.WriteString("\x1b[0;" + current + "m")
					}
				}
				j++
				continue
			}
			b.WriteByte(line[j])
		}
		if color {
			b.WriteString(reset)
		}
		out[i] = b.String()
	}
	return out
}

// marker reports whether s[i:] starts with "$1".."$9", returning a 0-based index.
func marker(s string, i int) (int, bool) {
	if s[i] == '$' && i+1 < len(s) && s[i+1] >= '1' && s[i+1] <= '9' {
		return int(s[i+1] - '1'), true
	}
	return 0, false
}

func visibleWidth(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		if _, ok := marker(line, i); ok {
			i++
			continue
		}
		n++
	}
	return n
}
