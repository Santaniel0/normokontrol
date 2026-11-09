// Package format holds Word's formatting vocabulary (units, enums, property
// sets, styles) and resolves the effective formatting of paragraphs and runs.
package format

import (
	"fmt"
	"math"
	"strings"
)

// Twips are 1/20 of a point: Word stores page sizes, margins and indents in them.
type Twips int

const twipsPerInch = 1440

// Millimeters converts to millimetres.
func (t Twips) Millimeters() float64 { return float64(t) * 25.4 / twipsPerInch }

// Centimeters converts to centimetres.
func (t Twips) Centimeters() float64 { return t.Millimeters() / 10 }

// TwipsFromMillimeters rounds a length in millimetres to twips.
func TwipsFromMillimeters(mm float64) Twips {
	return Twips(math.Round(mm * twipsPerInch / 25.4))
}

// HalfPoints is the unit of w:sz: font size 14 pt is stored as 28.
type HalfPoints int

// Points converts to typographic points.
func (h HalfPoints) Points() float64 { return float64(h) / 2 }

// Alignment is the paragraph justification (w:jc).
type Alignment int

const (
	AlignLeft Alignment = iota + 1
	AlignCenter
	AlignRight
	AlignJustify
)

func (a Alignment) String() string {
	switch a {
	case AlignLeft:
		return "left"
	case AlignCenter:
		return "center"
	case AlignRight:
		return "right"
	case AlignJustify:
		return "justify"
	}
	return fmt.Sprintf("Alignment(%d)", int(a))
}

// ParseJc maps a w:jc value to an Alignment. Bidi-aware "start"/"end" are
// treated as left/right because the checker targets left-to-right Russian text.
func ParseJc(v string) (Alignment, bool) {
	switch v {
	case "left", "start":
		return AlignLeft, true
	case "center":
		return AlignCenter, true
	case "right", "end":
		return AlignRight, true
	case "both", "distribute", "lowKashida", "mediumKashida", "highKashida", "thaiDistribute":
		return AlignJustify, true
	}
	return 0, false
}

// UnmarshalText lets profiles spell alignment as left/center/right/justify.
func (a *Alignment) UnmarshalText(b []byte) error {
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "left":
		*a = AlignLeft
	case "center":
		*a = AlignCenter
	case "right":
		*a = AlignRight
	case "justify", "both":
		*a = AlignJustify
	case "":
		*a = 0
	default:
		return fmt.Errorf("unknown alignment %q (want left, center, right or justify)", b)
	}
	return nil
}

// LineRule says how w:spacing/@w:line is interpreted.
type LineRule int

const (
	LineAuto    LineRule = iota + 1 // value in 240ths of a line
	LineExact                       // value in twips
	LineAtLeast                     // value in twips
)

func (r LineRule) String() string {
	switch r {
	case LineAuto:
		return "auto"
	case LineExact:
		return "exact"
	case LineAtLeast:
		return "atLeast"
	}
	return fmt.Sprintf("LineRule(%d)", int(r))
}

// LineSpacing is w:spacing line + lineRule taken together: Word always writes
// them as a pair, so they are inherited as one property.
type LineSpacing struct {
	Value int
	Rule  LineRule
}

// Multiple returns the spacing as a multiple of a single line (1.5 for
// "полуторный"); ok is false for exact and at-least spacing.
func (l LineSpacing) Multiple() (m float64, ok bool) {
	if l.Rule != LineAuto {
		return 0, false
	}
	return float64(l.Value) / 240, true
}

// ThemeFont names a font slot in the document theme (w:asciiTheme etc.).
type ThemeFont int

const (
	ThemeMinor ThemeFont = iota + 1
	ThemeMajor
)

// ParseThemeFont maps minorHAnsi, majorAscii and the like onto the two theme
// slots; the script part of the name does not matter for Latin and Cyrillic.
func ParseThemeFont(v string) (ThemeFont, bool) {
	switch {
	case strings.HasPrefix(v, "minor"):
		return ThemeMinor, true
	case strings.HasPrefix(v, "major"):
		return ThemeMajor, true
	}
	return 0, false
}

func (t ThemeFont) String() string {
	switch t {
	case ThemeMinor:
		return "minorHAnsi"
	case ThemeMajor:
		return "majorHAnsi"
	}
	return fmt.Sprintf("ThemeFont(%d)", int(t))
}
