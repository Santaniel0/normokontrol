package format

// Opt is a property that a given level of the cascade may or may not set.
type Opt[T comparable] struct {
	Val T
	Set bool
}

// Some returns a set Opt.
func Some[T comparable](v T) Opt[T] { return Opt[T]{Val: v, Set: true} }

// FontRef is either a literal font name or a reference to a theme font slot.
// When both w:hAnsi and w:hAnsiTheme are present the theme wins, as in Word.
type FontRef struct {
	Name  string
	Theme ThemeFont
}

// ParaProps are paragraph properties as written at one level (w:pPr).
type ParaProps struct {
	Align           Opt[Alignment]
	IndLeft         Opt[Twips]
	IndRight        Opt[Twips]
	FirstLine       Opt[Twips] // negative means a hanging indent
	Line            Opt[LineSpacing]
	Before          Opt[Twips]
	After           Opt[Twips]
	OutlineLevel    Opt[int]
	PageBreakBefore Opt[bool]
	Numbered        Opt[bool]
}

// RunProps are character properties as written at one level (w:rPr).
type RunProps struct {
	FontASCII Opt[FontRef]
	FontHAnsi Opt[FontRef]
	Size      Opt[HalfPoints]
	Bold      Opt[bool]
	Italic    Opt[bool]
	Caps      Opt[bool]
	Color     Opt[string]
}

// StyleID is the w:styleId of a style. Russian Word writes ids like "a" or
// "1", so rules must look at Style.Name, which keeps the English built-in name.
type StyleID string

// StyleType is w:style/@w:type.
type StyleType int

const (
	StyleParagraph StyleType = iota + 1
	StyleCharacter
	StyleTable
	StyleNumbering
)

func (t StyleType) String() string {
	switch t {
	case StyleParagraph:
		return "paragraph"
	case StyleCharacter:
		return "character"
	case StyleTable:
		return "table"
	case StyleNumbering:
		return "numbering"
	}
	return "unknown"
}

// Style is one w:style definition.
type Style struct {
	ID      StyleID
	Name    string
	Type    StyleType
	BasedOn StyleID
	Default bool
	Para    ParaProps
	Run     RunProps
}

// StyleSheet is the parsed styles.xml.
type StyleSheet struct {
	DefaultPara ParaProps // w:pPrDefault
	DefaultRun  RunProps  // w:rPrDefault
	Styles      map[StyleID]Style
}

// Theme carries the Latin typefaces of the theme font scheme.
type Theme struct {
	Major string
	Minor string
}

func (t Theme) font(slot ThemeFont) string {
	if slot == ThemeMajor {
		return t.Major
	}
	return t.Minor
}
