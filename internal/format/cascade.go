package format

import (
	"fmt"
	"sort"
)

// Source is the level of the cascade that supplied a resolved value.
type Source int

const (
	// SourceBuiltin means nothing in the file sets the value and Word's own
	// default applies (Times New Roman 10 pt, single spacing, no indents).
	SourceBuiltin Source = iota + 1
	SourceDocDefaults
	SourceParagraphStyle
	SourceCharacterStyle
	SourceDirect
)

func (s Source) String() string {
	switch s {
	case SourceBuiltin:
		return "builtin"
	case SourceDocDefaults:
		return "docDefaults"
	case SourceParagraphStyle:
		return "paragraphStyle"
	case SourceCharacterStyle:
		return "characterStyle"
	case SourceDirect:
		return "direct"
	}
	return fmt.Sprintf("Source(%d)", int(s))
}

// Origin records where a resolved value came from, so a report can say
// "fix the «Обычный» style" instead of listing forty paragraphs.
type Origin struct {
	Source    Source
	StyleID   StyleID
	StyleName string
	Theme     ThemeFont // set when a font name was taken from the theme
}

// Resolved is an effective value together with its origin.
type Resolved[T comparable] struct {
	Val  T
	From Origin
}

// ResolvedPara is the effective formatting of a paragraph.
type ResolvedPara struct {
	StyleID         StyleID
	StyleName       string
	Align           Resolved[Alignment]
	IndLeft         Resolved[Twips]
	IndRight        Resolved[Twips]
	FirstLine       Resolved[Twips]
	Line            Resolved[LineSpacing]
	Before          Resolved[Twips]
	After           Resolved[Twips]
	OutlineLevel    Resolved[int] // -1 when the paragraph is not part of the outline
	PageBreakBefore Resolved[bool]
	Numbered        Resolved[bool]
}

// ResolvedRun is the effective formatting of a run. Fonts are already mapped
// through the theme; an empty name means the theme part was missing.
type ResolvedRun struct {
	FontASCII Resolved[string]
	FontHAnsi Resolved[string]
	Size      Resolved[HalfPoints]
	Bold      Resolved[bool]
	Italic    Resolved[bool]
	Caps      Resolved[bool]
	Color     Resolved[string]
}

const maxStyleDepth = 32

// Resolver applies the cascade docDefaults → paragraph style chain →
// character style chain → direct formatting. Table styles and numbering
// definitions are not part of it; callers treat table cells and list items
// separately.
type Resolver struct {
	sheet   StyleSheet
	theme   Theme
	defPara StyleID
	defChar StyleID
}

// NewResolver prepares a resolver for one document.
func NewResolver(sheet StyleSheet, theme Theme) *Resolver {
	r := &Resolver{sheet: sheet, theme: theme}
	r.defPara = defaultStyleID(sheet, StyleParagraph)
	r.defChar = defaultStyleID(sheet, StyleCharacter)
	return r
}

// defaultStyleID picks the w:default style of a type. A broken file may mark
// several; taking the smallest id keeps the result deterministic.
func defaultStyleID(sheet StyleSheet, t StyleType) StyleID {
	var ids []StyleID
	for id, st := range sheet.Styles {
		if st.Default && st.Type == t {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids[0]
}

// Style returns a style by id.
func (r *Resolver) Style(id StyleID) (Style, bool) {
	st, ok := r.sheet.Styles[id]
	return st, ok
}

// chain returns the style followed by its basedOn ancestors, root first.
// Cycles and dangling basedOn references simply end the chain.
func (r *Resolver) chain(id StyleID, t StyleType) []Style {
	var rev []Style
	seen := make(map[StyleID]bool)
	for id != "" && !seen[id] && len(rev) < maxStyleDepth {
		st, ok := r.sheet.Styles[id]
		if !ok || st.Type != t {
			break
		}
		seen[id] = true
		rev = append(rev, st)
		id = st.BasedOn
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func (r *Resolver) paraStyleID(id StyleID) StyleID {
	if st, ok := r.sheet.Styles[id]; ok && st.Type == StyleParagraph {
		return id
	}
	return r.defPara
}

func (r *Resolver) charStyleID(id StyleID) StyleID {
	if st, ok := r.sheet.Styles[id]; ok && st.Type == StyleCharacter {
		return id
	}
	return r.defChar
}

func set[T comparable](dst *Resolved[T], src Opt[T], o Origin) {
	if src.Set {
		*dst = Resolved[T]{Val: src.Val, From: o}
	}
}

func applyPara(dst *ResolvedPara, p ParaProps, o Origin) {
	set(&dst.Align, p.Align, o)
	set(&dst.IndLeft, p.IndLeft, o)
	set(&dst.IndRight, p.IndRight, o)
	set(&dst.FirstLine, p.FirstLine, o)
	set(&dst.Line, p.Line, o)
	set(&dst.Before, p.Before, o)
	set(&dst.After, p.After, o)
	set(&dst.OutlineLevel, p.OutlineLevel, o)
	set(&dst.PageBreakBefore, p.PageBreakBefore, o)
	set(&dst.Numbered, p.Numbered, o)
}

// Paragraph resolves paragraph properties for a paragraph with the given
// w:pStyle (possibly empty) and direct w:pPr.
func (r *Resolver) Paragraph(styleID StyleID, direct ParaProps) ResolvedPara {
	b := Origin{Source: SourceBuiltin}
	res := ResolvedPara{
		Align:           Resolved[Alignment]{AlignLeft, b},
		IndLeft:         Resolved[Twips]{0, b},
		IndRight:        Resolved[Twips]{0, b},
		FirstLine:       Resolved[Twips]{0, b},
		Line:            Resolved[LineSpacing]{LineSpacing{240, LineAuto}, b},
		Before:          Resolved[Twips]{0, b},
		After:           Resolved[Twips]{0, b},
		OutlineLevel:    Resolved[int]{-1, b},
		PageBreakBefore: Resolved[bool]{false, b},
		Numbered:        Resolved[bool]{false, b},
	}
	applyPara(&res, r.sheet.DefaultPara, Origin{Source: SourceDocDefaults})
	id := r.paraStyleID(styleID)
	for _, st := range r.chain(id, StyleParagraph) {
		applyPara(&res, st.Para, Origin{Source: SourceParagraphStyle, StyleID: st.ID, StyleName: st.Name})
	}
	applyPara(&res, direct, Origin{Source: SourceDirect})
	if st, ok := r.sheet.Styles[id]; ok {
		res.StyleID, res.StyleName = st.ID, st.Name
	}
	return res
}

// runLevel accumulates RunProps with origins before fonts are looked up in
// the theme.
type runLevel struct {
	ascii, hAnsi       Resolved[FontRef]
	size               Resolved[HalfPoints]
	bold, italic, caps Resolved[bool]
	color              Resolved[string]
}

func applyRunLevel(dst *runLevel, p RunProps, o Origin, toggles bool) {
	set(&dst.ascii, p.FontASCII, o)
	set(&dst.hAnsi, p.FontHAnsi, o)
	set(&dst.size, p.Size, o)
	set(&dst.color, p.Color, o)
	if toggles {
		set(&dst.bold, p.Bold, o)
		set(&dst.italic, p.Italic, o)
		set(&dst.caps, p.Caps, o)
	}
}

// Run resolves character properties of a run in a paragraph with the given
// paragraph style, the run's w:rStyle and its direct w:rPr.
//
// Bold, italic and caps are toggle properties (ECMA-376 17.7.3): a character
// style that sets bold on text whose paragraph style is already bold turns
// bold off. Inside one basedOn chain the derived style simply overrides, and
// direct formatting always sets the value outright.
func (r *Resolver) Run(paraStyle, charStyle StyleID, direct RunProps) ResolvedRun {
	b := Origin{Source: SourceBuiltin}
	lv := runLevel{
		ascii:  Resolved[FontRef]{FontRef{Name: "Times New Roman"}, b},
		hAnsi:  Resolved[FontRef]{FontRef{Name: "Times New Roman"}, b},
		size:   Resolved[HalfPoints]{20, b},
		bold:   Resolved[bool]{false, b},
		italic: Resolved[bool]{false, b},
		caps:   Resolved[bool]{false, b},
		color:  Resolved[string]{"auto", b},
	}
	applyRunLevel(&lv, r.sheet.DefaultRun, Origin{Source: SourceDocDefaults}, true)
	for _, st := range r.chain(r.paraStyleID(paraStyle), StyleParagraph) {
		applyRunLevel(&lv, st.Run, Origin{Source: SourceParagraphStyle, StyleID: st.ID, StyleName: st.Name}, true)
	}
	var ch runLevel
	for _, st := range r.chain(r.charStyleID(charStyle), StyleCharacter) {
		o := Origin{Source: SourceCharacterStyle, StyleID: st.ID, StyleName: st.Name}
		applyRunLevel(&lv, st.Run, o, false)
		applyRunLevel(&ch, st.Run, o, true)
	}
	lv.bold = toggle(lv.bold, ch.bold)
	lv.italic = toggle(lv.italic, ch.italic)
	lv.caps = toggle(lv.caps, ch.caps)
	applyRunLevel(&lv, direct, Origin{Source: SourceDirect}, true)

	return ResolvedRun{
		FontASCII: r.font(lv.ascii),
		FontHAnsi: r.font(lv.hAnsi),
		Size:      lv.size,
		Bold:      lv.bold,
		Italic:    lv.italic,
		Caps:      lv.caps,
		Color:     lv.color,
	}
}

func toggle(base, char Resolved[bool]) Resolved[bool] {
	if char.From.Source == 0 {
		return base
	}
	if base.From.Source == SourceParagraphStyle {
		return Resolved[bool]{Val: base.Val != char.Val, From: char.From}
	}
	return char
}

func (r *Resolver) font(f Resolved[FontRef]) Resolved[string] {
	if f.Val.Theme == 0 {
		return Resolved[string]{Val: f.Val.Name, From: f.From}
	}
	o := f.From
	o.Theme = f.Val.Theme
	return Resolved[string]{Val: r.theme.font(f.Val.Theme), From: o}
}
