package render

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogosWellFormed(t *testing.T) {
	for name, l := range Logos {
		if len(l.Lines) == 0 || len(l.Colors) == 0 {
			t.Errorf("%s: empty logo", name)
		}
		for i, line := range l.Lines {
			for j := 0; j < len(line); j++ {
				if n, ok := marker(line, j); ok && n >= len(l.Colors) {
					t.Errorf("%s line %d: color $%d undefined", name, i, n+1)
				}
			}
		}
	}
}

func TestRenderAlignment(t *testing.T) {
	logo := Logos["hetzner"]
	var buf bytes.Buffer
	card := Card{
		Title: "example.com",
		Logo:  "hetzner",
		Rows:  []Row{{Key: "NS", Values: []string{"a", "b"}}},
	}
	if err := Render(&buf, card, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(logo.Lines) {
		t.Fatalf("got %d lines, want %d", len(lines), len(logo.Lines))
	}
	col := visibleWidth(logo.Lines[0]) + gap
	want := []string{"example.com", "-----------", "NS: a", "    b"}
	for i, w := range want {
		if got := lines[i][col:]; got != w {
			t.Errorf("line %d info = %q, want %q", i, got, w)
		}
	}
}

func TestRenderColor(t *testing.T) {
	var plain, colored bytes.Buffer
	card := Card{Title: "x", Logo: "unknown-falls-back"}
	Render(&plain, card, false)
	Render(&colored, card, true)
	if strings.Contains(plain.String(), "\x1b") || strings.Contains(plain.String(), "$1") {
		t.Errorf("plain output has escapes or markers: %q", plain.String())
	}
	if !strings.Contains(colored.String(), "38;5;45m") {
		t.Error("colored output should use the generic logo color")
	}
}

func TestRenderBadRow(t *testing.T) {
	var buf bytes.Buffer
	Render(&buf, Card{Title: "x", Logo: "none", Rows: []Row{{Key: "IPv6", Values: []string{"✗ none"}, Bad: true}}}, true)
	if !strings.Contains(buf.String(), "\x1b[31m✗ none\x1b[0m") {
		t.Errorf("bad row not red: %q", buf.String())
	}
}
