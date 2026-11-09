// Package model turns parsed paragraphs into a document the rules can reason
// about: every paragraph gets its effective formatting, a role (heading, body
// text, caption, table cell...) and a place in the outline.
package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Santaniel0/normokontrol/internal/format"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
)

// Role is what a paragraph is for.
type Role int

const (
	RoleBody Role = iota + 1
	RoleHeading
	RoleEmpty
	RoleFigure  // paragraph holding an image or other drawing
	RoleCaption // "Рисунок 1 – ..." or "Таблица 1 – ..."
	RoleTableCell
	RoleListItem
	RoleTOC
	RoleBibEntry
	RoleFormula
	RoleCode
	RoleFrontMatter // title page and anything else before the first heading
)

var roleNames = map[Role]string{
	RoleBody: "body", RoleHeading: "heading", RoleEmpty: "empty", RoleFigure: "figure",
	RoleCaption: "caption", RoleTableCell: "table-cell", RoleListItem: "list-item", RoleTOC: "toc",
	RoleBibEntry: "bib-entry", RoleFormula: "formula", RoleCode: "code", RoleFrontMatter: "front-matter",
}

func (r Role) String() string {
	if s, ok := roleNames[r]; ok {
		return s
	}
	return fmt.Sprintf("Role(%d)", int(r))
}

// Region is the structural part of the document a paragraph belongs to.
type Region int

const (
	RegionFront Region = iota + 1
	RegionMain
	RegionAppendix
)

// FontUse is the font that most of a paragraph's visible characters use.
type FontUse struct {
	Name  format.Resolved[string]
	Size  format.Resolved[format.HalfPoints]
	Chars int
}

// Paragraph is a classified paragraph.
type Paragraph struct {
	Index   int
	Src     *ooxml.Paragraph
	Text    string
	Role    Role
	Region  Region
	Level   int  // heading level, 1-based
	Styled  bool // heading recognised by its style or outline level, not guessed
	Bold    bool // every visible character is bold
	Caption *Caption
	Format  format.ResolvedPara
	Font    FontUse   // the most used font and size
	Fonts   []FontUse // every font and size of visible text, most used first
	Heading int       // index of the governing heading, -1 before the first one
	Ordinal int       // 1-based position among non-empty paragraphs under that heading
}

// Document is the classified document.
type Document struct {
	Paras        []*Paragraph
	Sections     []ooxml.Section
	SectionStart []int // index of the first paragraph of each section, -1 if empty
	Tables       int
}

// Options carries the heading names a profile knows about, so that headings
// typed by hand without a heading style are still recognised.
type Options struct {
	Structural   []string
	Bibliography []string
}

// Built-in names in addition to the profile's. ГОСТ 7.32-2017, 6.2.1.
var (
	tocNames        = []string{"СОДЕРЖАНИЕ", "ОГЛАВЛЕНИЕ"}
	builtinStruct   = []string{"СПИСОК ИСПОЛНИТЕЛЕЙ", "РЕФЕРАТ", "АННОТАЦИЯ", "СОДЕРЖАНИЕ", "ОГЛАВЛЕНИЕ", "ТЕРМИНЫ И ОПРЕДЕЛЕНИЯ", "ПЕРЕЧЕНЬ СОКРАЩЕНИЙ И ОБОЗНАЧЕНИЙ", "ВВЕДЕНИЕ", "ЗАКЛЮЧЕНИЕ"}
	builtinBib      = []string{"СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ", "СПИСОК ЛИТЕРАТУРЫ", "СПИСОК ИСПОЛЬЗОВАННОЙ ЛИТЕРАТУРЫ", "БИБЛИОГРАФИЧЕСКИЙ СПИСОК", "СПИСОК ИСТОЧНИКОВ"}
	appendixPrefix  = "ПРИЛОЖЕНИЕ"
	numberedHeading = regexp.MustCompile(`^(\d+(?:\.\d+){0,3})\.?\s+\S`)
	listMarker      = regexp.MustCompile(`^(?:[-–—•·▪]\s|[а-яa-z]\)\s|\d+\)\s)`)
	headingStyle    = regexp.MustCompile(`(?i)^(?:heading|заголовок)\s*([1-9])$`)
	monospace       = map[string]bool{"Courier New": true, "Consolas": true, "Courier": true, "Lucida Console": true,
		"Cascadia Code": true, "Cascadia Mono": true, "JetBrains Mono": true, "PT Mono": true,
		"Source Code Pro": true, "Liberation Mono": true, "DejaVu Sans Mono": true, "Menlo": true}
)

// Normalize prepares heading text for comparison: upper case, single spaces,
// no trailing period.
func Normalize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, ".")
	return strings.ToUpper(strings.ReplaceAll(s, "ё", "е"))
}

type nameSet map[string]bool

func newNameSet(lists ...[]string) nameSet {
	s := make(nameSet)
	for _, l := range lists {
		for _, n := range l {
			s[Normalize(n)] = true
		}
	}
	return s
}

// Build classifies every paragraph of a loaded document.
func Build(doc *ooxml.Document, opt Options) *Document {
	res := format.NewResolver(doc.Styles, doc.Theme)
	structural := newNameSet(builtinStruct, builtinBib, opt.Structural, opt.Bibliography)
	bib := newNameSet(builtinBib, opt.Bibliography)
	toc := newNameSet(tocNames)

	d := &Document{Sections: doc.Body.Sections, Tables: doc.Body.Tables}
	d.SectionStart = make([]int, len(d.Sections))
	for i := range d.SectionStart {
		d.SectionStart[i] = -1
	}
	for i, src := range doc.Body.Paragraphs {
		p := &Paragraph{Index: i, Src: src, Text: src.Text(), Heading: -1}
		p.Format = res.Paragraph(src.StyleID, src.Props)
		p.Fonts, p.Bold = fontUses(res, p.Format.StyleID, src)
		if len(p.Fonts) > 0 {
			p.Font = p.Fonts[0]
		}
		p.Role, p.Level, p.Styled = classify(p, structural)
		if src.Section < len(d.SectionStart) && d.SectionStart[src.Section] < 0 {
			d.SectionStart[src.Section] = i
		}
		d.Paras = append(d.Paras, p)
	}
	d.markCaptions()
	d.markRegions(toc, bib)
	d.number()
	return d
}

func classify(p *Paragraph, structural nameSet) (Role, int, bool) {
	text := strings.TrimSpace(p.Text)
	style := strings.ToLower(p.Format.StyleName)
	switch {
	case p.Src.Table > 0:
		return RoleTableCell, 0, false
	case strings.HasPrefix(style, "toc "):
		return RoleTOC, 0, false
	case p.Src.Drawing && letters(text) < 3:
		return RoleFigure, 0, false
	case p.Src.Math && letters(text) < 3:
		return RoleFormula, 0, false
	case text == "":
		return RoleEmpty, 0, false
	}
	if lvl := p.Format.OutlineLevel.Val; lvl >= 0 {
		return RoleHeading, lvl + 1, true
	}
	if m := headingStyle.FindStringSubmatch(p.Format.StyleName); m != nil {
		return RoleHeading, int(m[1][0] - '0'), true
	}
	if utf8.RuneCountInString(text) <= 150 {
		norm := Normalize(text)
		if structural[norm] || strings.HasPrefix(norm, appendixPrefix) && utf8.RuneCountInString(norm) < 80 {
			return RoleHeading, 1, false
		}
		if m := numberedHeading.FindStringSubmatch(text); m != nil && p.Bold && !strings.HasSuffix(text, ":") {
			return RoleHeading, strings.Count(m[1], ".") + 1, false
		}
	}
	if p.Format.Numbered.Val || listMarker.MatchString(text) {
		return RoleListItem, 0, false
	}
	if monospace[p.Font.Name.Val] {
		return RoleCode, 0, false
	}
	return RoleBody, 0, false
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

type fontKey struct {
	name string
	size format.HalfPoints
}

// fontUses lists the fonts and sizes of visible characters, most used first.
// Word draws Cyrillic with the w:hAnsi font and plain ASCII with w:ascii, so
// the slot is chosen per run. bold reports whether every visible run is bold.
func fontUses(res *format.Resolver, paraStyle format.StyleID, src *ooxml.Paragraph) (uses []FontUse, bold bool) {
	index := map[fontKey]int{}
	bold = true
	for _, r := range src.Runs {
		n := letters(r.Text)
		if n == 0 {
			continue
		}
		rr := res.Run(paraStyle, r.StyleID, r.Props)
		font := rr.FontASCII
		for _, c := range r.Text {
			if c > unicode.MaxASCII && unicode.IsLetter(c) {
				font = rr.FontHAnsi
				break
			}
		}
		k := fontKey{font.Val, rr.Size.Val}
		i, ok := index[k]
		if !ok {
			i = len(uses)
			index[k] = i
			uses = append(uses, FontUse{Name: font, Size: rr.Size})
		}
		uses[i].Chars += n
		bold = bold && rr.Bold.Val
	}
	sort.SliceStable(uses, func(i, j int) bool { return uses[i].Chars > uses[j].Chars })
	return uses, bold && len(uses) > 0
}

func (d *Document) markCaptions() {
	for i, p := range d.Paras {
		if p.Role != RoleBody && p.Role != RoleListItem {
			continue
		}
		c, ok := ParseCaption(p.Text)
		if !ok {
			continue
		}
		adjacent := c.Kind == CaptionFigure && d.neighbour(i, -1, RoleFigure) ||
			c.Kind == CaptionTable && d.neighbour(i, +1, RoleTableCell)
		if c.Sep != "" || c.Continuation || adjacent || strings.EqualFold(p.Format.StyleName, "caption") {
			p.Role, p.Caption = RoleCaption, &c
		}
	}
}

// neighbour reports whether the nearest non-empty paragraph in direction dir
// has the given role.
func (d *Document) neighbour(i, dir int, role Role) bool {
	for j, steps := i+dir, 0; j >= 0 && j < len(d.Paras) && steps < 3; j, steps = j+dir, steps+1 {
		if d.Paras[j].Role != RoleEmpty {
			return d.Paras[j].Role == role
		}
	}
	return false
}

func (d *Document) markRegions(toc, bib nameSet) {
	region := RegionFront
	inTOC, inBib := false, false
	for _, p := range d.Paras {
		if p.Role == RoleHeading {
			norm := Normalize(p.Text)
			if region == RegionFront {
				region = RegionMain
			}
			if strings.HasPrefix(norm, appendixPrefix) {
				region = RegionAppendix
			}
			inTOC, inBib = toc[norm], bib[norm]
			p.Region = region
			continue
		}
		p.Region = region
		switch {
		case region == RegionFront && p.Role != RoleEmpty:
			p.Role = RoleFrontMatter
		case inTOC && (p.Role == RoleBody || p.Role == RoleListItem):
			p.Role = RoleTOC
		case inBib && (p.Role == RoleBody || p.Role == RoleListItem):
			p.Role = RoleBibEntry
		}
	}
}

func (d *Document) number() {
	heading, ordinal := -1, 0
	for i, p := range d.Paras {
		if p.Role == RoleHeading {
			heading, ordinal = i, 0
			p.Heading = i
			continue
		}
		p.Heading = heading
		if p.Role != RoleEmpty {
			ordinal++
			p.Ordinal = ordinal
		}
	}
}

// BibEntries returns the reference list entries in order.
func (d *Document) BibEntries() []*Paragraph {
	var out []*Paragraph
	for _, p := range d.Paras {
		if p.Role == RoleBibEntry {
			out = append(out, p)
		}
	}
	return out
}

// Location is a human-oriented position of a paragraph: page numbers are
// unknown without layout, so the nearest heading and an ordinal are used.
type Location struct {
	Para    int // -1 for findings about the whole document
	Heading string
	Ordinal int
	Region  Region
	Role    Role
	Snippet string
}

// Locate describes paragraph i.
func (d *Document) Locate(i int) Location {
	if i < 0 || i >= len(d.Paras) {
		return Location{Para: -1}
	}
	p := d.Paras[i]
	loc := Location{Para: i, Ordinal: p.Ordinal, Region: p.Region, Role: p.Role, Snippet: Snippet(p.Text, 50)}
	if p.Heading >= 0 && p.Heading != i {
		loc.Heading = Snippet(d.Paras[p.Heading].Text, 60)
	}
	return loc
}

// Snippet shortens text to n runes on a word boundary where possible.
func Snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	if i := strings.LastIndex(string(r), " "); i > n/2 {
		return string(r)[:i] + "…"
	}
	return string(r) + "…"
}
