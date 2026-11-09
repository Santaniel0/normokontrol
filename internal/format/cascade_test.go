package format

import (
	"math"
	"testing"
)

// sheet builds a style sheet that looks like Russian Word's: Normal has id
// "a", headings are "1", "2", the default paragraph font is "a0".
func sheet(styles ...Style) StyleSheet {
	s := StyleSheet{Styles: make(map[StyleID]Style)}
	for _, st := range styles {
		s.Styles[st.ID] = st
	}
	return s
}

var (
	normalTNR14 = Style{ID: "a", Name: "Normal", Type: StyleParagraph, Default: true,
		Run: RunProps{FontHAnsi: Some(FontRef{Name: "Times New Roman"}), Size: Some(HalfPoints(28))}}
	normalBare  = Style{ID: "a", Name: "Normal", Type: StyleParagraph, Default: true}
	defaultFont = Style{ID: "a0", Name: "Default Paragraph Font", Type: StyleCharacter, Default: true}
	heading1    = Style{ID: "1", Name: "heading 1", Type: StyleParagraph, BasedOn: "a",
		Para: ParaProps{OutlineLevel: Some(0), Align: Some(AlignCenter)},
		Run:  RunProps{Bold: Some(true)}}
	strong = Style{ID: "af", Name: "Strong", Type: StyleCharacter, BasedOn: "a0",
		Run: RunProps{Bold: Some(true), FontHAnsi: Some(FontRef{Name: "Arial"})}}
)

func TestRunCascade(t *testing.T) {
	theme := Theme{Major: "Calibri Light", Minor: "Calibri"}
	tests := []struct {
		name       string
		sheet      StyleSheet
		theme      Theme
		paraStyle  StyleID
		charStyle  StyleID
		direct     RunProps
		wantFont   string
		wantFontBy Source
		wantStyle  StyleID
		wantTheme  ThemeFont
		wantSize   HalfPoints
		wantSizeBy Source
	}{
		{
			name:     "nothing set falls back to Word builtin",
			sheet:    sheet(),
			wantFont: "Times New Roman", wantFontBy: SourceBuiltin,
			wantSize: 20, wantSizeBy: SourceBuiltin,
		},
		{
			name: "docDefaults theme font is looked up in the theme",
			sheet: StyleSheet{
				DefaultRun: RunProps{FontHAnsi: Some(FontRef{Theme: ThemeMinor}), Size: Some(HalfPoints(22))},
				Styles:     map[StyleID]Style{"a": normalBare},
			},
			theme:    theme,
			wantFont: "Calibri", wantFontBy: SourceDocDefaults, wantTheme: ThemeMinor,
			wantSize: 22, wantSizeBy: SourceDocDefaults,
		},
		{
			name: "default paragraph style overrides docDefaults when pStyle is absent",
			sheet: StyleSheet{
				DefaultRun: RunProps{FontHAnsi: Some(FontRef{Theme: ThemeMinor}), Size: Some(HalfPoints(22))},
				Styles:     map[StyleID]Style{"a": normalTNR14},
			},
			theme:    theme,
			wantFont: "Times New Roman", wantFontBy: SourceParagraphStyle, wantStyle: "a",
			wantSize: 28, wantSizeBy: SourceParagraphStyle,
		},
		{
			name:      "unknown pStyle behaves like the default style",
			sheet:     sheet(normalTNR14),
			paraStyle: "missing",
			wantFont:  "Times New Roman", wantFontBy: SourceParagraphStyle, wantStyle: "a",
			wantSize: 28, wantSizeBy: SourceParagraphStyle,
		},
		{
			name:      "property inherited through basedOn keeps the ancestor as origin",
			sheet:     sheet(normalTNR14, heading1),
			paraStyle: "1",
			wantFont:  "Times New Roman", wantFontBy: SourceParagraphStyle, wantStyle: "a",
			wantSize: 28, wantSizeBy: SourceParagraphStyle,
		},
		{
			name: "derived style overrides its base",
			sheet: sheet(normalTNR14, Style{ID: "x", Name: "Body", Type: StyleParagraph, BasedOn: "a",
				Run: RunProps{Size: Some(HalfPoints(24))}}),
			paraStyle: "x",
			wantFont:  "Times New Roman", wantFontBy: SourceParagraphStyle, wantStyle: "a",
			wantSize: 24, wantSizeBy: SourceParagraphStyle,
		},
		{
			name:      "character style beats paragraph style",
			sheet:     sheet(normalTNR14, defaultFont, strong),
			charStyle: "af",
			wantFont:  "Arial", wantFontBy: SourceCharacterStyle, wantStyle: "af",
			wantSize: 28, wantSizeBy: SourceParagraphStyle,
		},
		{
			name:      "direct formatting beats everything",
			sheet:     sheet(normalTNR14, defaultFont, strong),
			charStyle: "af",
			direct:    RunProps{FontHAnsi: Some(FontRef{Name: "Courier New"}), Size: Some(HalfPoints(24))},
			wantFont:  "Courier New", wantFontBy: SourceDirect,
			wantSize: 24, wantSizeBy: SourceDirect,
		},
		{
			name:     "theme attribute wins over a literal name at the same level",
			sheet:    sheet(normalBare),
			theme:    theme,
			direct:   RunProps{FontHAnsi: Some(FontRef{Name: "Arial", Theme: ThemeMajor})},
			wantFont: "Calibri Light", wantFontBy: SourceDirect, wantTheme: ThemeMajor,
			wantSize: 20, wantSizeBy: SourceBuiltin,
		},
		{
			name: "missing theme part leaves the name unknown",
			sheet: StyleSheet{DefaultRun: RunProps{FontHAnsi: Some(FontRef{Theme: ThemeMinor})},
				Styles: map[StyleID]Style{"a": normalBare}},
			wantFont: "", wantFontBy: SourceDocDefaults, wantTheme: ThemeMinor,
			wantSize: 20, wantSizeBy: SourceBuiltin,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResolver(tt.sheet, tt.theme).Run(tt.paraStyle, tt.charStyle, tt.direct)
			if r.FontHAnsi.Val != tt.wantFont || r.FontHAnsi.From.Source != tt.wantFontBy {
				t.Errorf("font = %q from %v, want %q from %v", r.FontHAnsi.Val, r.FontHAnsi.From.Source, tt.wantFont, tt.wantFontBy)
			}
			if r.FontHAnsi.From.StyleID != tt.wantStyle {
				t.Errorf("font style origin = %q, want %q", r.FontHAnsi.From.StyleID, tt.wantStyle)
			}
			if r.FontHAnsi.From.Theme != tt.wantTheme {
				t.Errorf("font theme slot = %v, want %v", r.FontHAnsi.From.Theme, tt.wantTheme)
			}
			if r.Size.Val != tt.wantSize || r.Size.From.Source != tt.wantSizeBy {
				t.Errorf("size = %d from %v, want %d from %v", r.Size.Val, r.Size.From.Source, tt.wantSize, tt.wantSizeBy)
			}
		})
	}
}

func TestBoldToggle(t *testing.T) {
	notBoldHeading := Style{ID: "2", Name: "heading 2", Type: StyleParagraph, BasedOn: "1",
		Run: RunProps{Bold: Some(false)}}
	tests := []struct {
		name      string
		paraStyle StyleID
		charStyle StyleID
		direct    RunProps
		want      bool
		wantBy    Source
	}{
		{"plain text", "a", "", RunProps{}, false, SourceBuiltin},
		{"bold paragraph style", "1", "", RunProps{}, true, SourceParagraphStyle},
		{"bold character style on plain text", "a", "af", RunProps{}, true, SourceCharacterStyle},
		{"bold character style on bold heading toggles off", "1", "af", RunProps{}, false, SourceCharacterStyle},
		{"derived style switches bold off without toggling", "2", "", RunProps{}, false, SourceParagraphStyle},
		{"direct bold=false wins", "1", "", RunProps{Bold: Some(false)}, false, SourceDirect},
		{"direct bold=true does not toggle", "1", "", RunProps{Bold: Some(true)}, true, SourceDirect},
	}
	r := NewResolver(sheet(normalTNR14, heading1, notBoldHeading, defaultFont, strong), Theme{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Run(tt.paraStyle, tt.charStyle, tt.direct).Bold
			if got.Val != tt.want || got.From.Source != tt.wantBy {
				t.Errorf("bold = %v from %v, want %v from %v", got.Val, got.From.Source, tt.want, tt.wantBy)
			}
		})
	}
}

func TestParagraphCascade(t *testing.T) {
	normal := Style{ID: "a", Name: "Normal", Type: StyleParagraph, Default: true,
		Para: ParaProps{
			Align:     Some(AlignJustify),
			FirstLine: Some(TwipsFromMillimeters(12.5)),
			Line:      Some(LineSpacing{360, LineAuto}),
		}}
	sh := StyleSheet{
		DefaultPara: ParaProps{After: Some(Twips(160)), Line: Some(LineSpacing{259, LineAuto})},
		Styles:      map[StyleID]Style{"a": normal, "1": heading1},
	}
	r := NewResolver(sh, Theme{})

	t.Run("default style over docDefaults", func(t *testing.T) {
		p := r.Paragraph("", ParaProps{})
		if p.StyleName != "Normal" {
			t.Errorf("style name = %q", p.StyleName)
		}
		if p.Line.Val != (LineSpacing{360, LineAuto}) || p.Line.From.StyleID != "a" {
			t.Errorf("line = %+v", p.Line)
		}
		if p.After.Val != 160 || p.After.From.Source != SourceDocDefaults {
			t.Errorf("after = %+v", p.After)
		}
		if p.OutlineLevel.Val != -1 {
			t.Errorf("outline = %d, want -1", p.OutlineLevel.Val)
		}
	})
	t.Run("heading inherits indent from Normal and sets its own outline", func(t *testing.T) {
		p := r.Paragraph("1", ParaProps{})
		if p.OutlineLevel.Val != 0 || p.OutlineLevel.From.StyleID != "1" {
			t.Errorf("outline = %+v", p.OutlineLevel)
		}
		if p.Align.Val != AlignCenter {
			t.Errorf("align = %v", p.Align.Val)
		}
		if p.FirstLine.From.StyleID != "a" {
			t.Errorf("first line origin = %+v", p.FirstLine.From)
		}
	})
	t.Run("direct hanging indent replaces the style first line", func(t *testing.T) {
		p := r.Paragraph("", ParaProps{FirstLine: Some(Twips(-360))})
		if p.FirstLine.Val != -360 || p.FirstLine.From.Source != SourceDirect {
			t.Errorf("first line = %+v", p.FirstLine)
		}
		if p.Align.From.StyleID != "a" {
			t.Errorf("alignment should still come from Normal, got %+v", p.Align.From)
		}
	})
}

func TestBasedOnCycleTerminates(t *testing.T) {
	sh := sheet(
		Style{ID: "x", Name: "X", Type: StyleParagraph, BasedOn: "y", Para: ParaProps{Align: Some(AlignRight)}},
		Style{ID: "y", Name: "Y", Type: StyleParagraph, BasedOn: "x", Para: ParaProps{FirstLine: Some(Twips(100))}},
	)
	p := NewResolver(sh, Theme{}).Paragraph("x", ParaProps{})
	if p.Align.Val != AlignRight || p.FirstLine.Val != 100 {
		t.Errorf("got align %v first line %d", p.Align.Val, p.FirstLine.Val)
	}
}

func TestUnits(t *testing.T) {
	if got := TwipsFromMillimeters(30); got != 1701 {
		t.Errorf("30 mm = %d twips, want 1701", got)
	}
	if got := Twips(709).Centimeters(); math.Abs(got-1.25) > 0.001 {
		t.Errorf("709 twips = %.4f cm, want 1.25", got)
	}
	if m, ok := (LineSpacing{360, LineAuto}).Multiple(); !ok || m != 1.5 {
		t.Errorf("360 auto = %v %v", m, ok)
	}
	if _, ok := (LineSpacing{360, LineExact}).Multiple(); ok {
		t.Error("exact spacing must not convert to a multiple")
	}
	var a Alignment
	if err := a.UnmarshalText([]byte("Justify")); err != nil || a != AlignJustify {
		t.Errorf("UnmarshalText = %v, %v", a, err)
	}
	if err := a.UnmarshalText([]byte("middle")); err == nil {
		t.Error("expected an error for an unknown alignment")
	}
}
