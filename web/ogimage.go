package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Preview image size recommended by Open Graph and X (Twitter) cards.
const (
	ogWidth   = 1200
	ogHeight  = 630
	ogPadding = 48
	ogMaxRows = 26   // longer output is cut off with "…"
	ogMaxSize = 30.0 // px; short output is not blown up beyond this
	ogLine    = 1.3  // line height in ems
)

var (
	ogBG      = color.RGBA{0x0d, 0x11, 0x17, 0xff}
	ogFG      = color.RGBA{0xe6, 0xed, 0xf3, 0xff}
	ogMuted   = color.RGBA{0x8b, 0x94, 0x9e, 0xff}
	ogRegular = mustFont(gomono.TTF)
	ogBold    = mustFont(gomonobold.TTF)
)

func mustFont(ttf []byte) *opentype.Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	return f
}

// glyphFallbacks replaces characters Go Mono lacks with close equivalents.
var glyphFallbacks = map[rune]rune{'✗': '×', '✓': '√'}

// renderOGImage draws dossier's colored terminal output as a PNG card,
// scaled to fit, with a "dossier <domain>" caption.
func renderOGImage(out []byte, domain string) ([]byte, error) {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > ogMaxRows {
		lines = append(lines[:ogMaxRows-1], "…")
	}
	cols := 1
	for _, l := range lines {
		cols = max(cols, utf8.RuneCountInString(stripANSI(l)))
	}

	// Go Mono's advance is 0.6 em. Fit the grid into the area above the
	// caption, which takes two lines.
	rows := float64(len(lines) + 2)
	size := math.Min(ogMaxSize, math.Min(
		float64(ogWidth-2*ogPadding)/(float64(cols)*0.6),
		float64(ogHeight-2*ogPadding)/(rows*ogLine),
	))
	regular, err := opentype.NewFace(ogRegular, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer regular.Close()
	bold, err := opentype.NewFace(ogBold, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer bold.Close()

	img := image.NewRGBA(image.Rect(0, 0, ogWidth, ogHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(ogBG), image.Point{}, draw.Src)

	adv := size * 0.6
	lineH := size * ogLine
	ascent := float64(regular.Metrics().Ascent.Ceil())
	// Center the block horizontally and vertically.
	blockW := float64(cols) * adv
	blockH := rows * lineH
	left := math.Max(ogPadding, (ogWidth-blockW)/2)
	top := math.Max(ogPadding, (ogHeight-blockH)/2)

	var buf sfnt.Buffer
	drawRune := func(face font.Face, f *opentype.Font, c color.Color, r rune, col, row int) {
		if r == ' ' {
			return
		}
		if fb, ok := glyphFallbacks[r]; ok {
			r = fb
		}
		if i, err := f.GlyphIndex(&buf, r); err != nil || i == 0 {
			r = '?'
		}
		d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face}
		// Place every glyph on the cell grid so columns line up exactly.
		d.Dot = fixed.P(int(math.Round(left+float64(col)*adv)), int(math.Round(top+float64(row)*lineH+ascent)))
		d.DrawString(string(r))
	}

	for row, line := range lines {
		col := 0
		for _, seg := range parseANSI(line) {
			face, f := font.Face(regular), ogRegular
			if seg.Bold {
				face, f = bold, ogBold
			}
			c := color.Color(ogFG)
			if seg.FG != "" {
				c = hexColor(seg.FG)
			}
			for _, r := range seg.Text {
				drawRune(face, f, c, r, col, row)
				col++
			}
		}
	}

	// Caption: "$ dossier <domain>" on the last line of the block.
	caption := "$ dossier " + domain
	for i, r := range caption {
		c := color.Color(ogMuted)
		if i == 0 {
			c = color.RGBA{0x3f, 0xb9, 0x50, 0xff}
		}
		drawRune(regular, ogRegular, c, r, utf8.RuneCountInString(caption[:i]), len(lines)+1)
	}

	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func stripANSI(s string) string {
	var b strings.Builder
	for _, seg := range parseANSI(s) {
		b.WriteString(seg.Text)
	}
	return b.String()
}

// hexColor parses "#rrggbb".
func hexColor(s string) color.Color {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return ogFG
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}
