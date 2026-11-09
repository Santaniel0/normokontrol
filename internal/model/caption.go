package model

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CaptionKind tells figure captions from table titles.
type CaptionKind int

const (
	CaptionFigure CaptionKind = iota + 1
	CaptionTable
)

// Caption is a parsed "Рисунок 3 – Схема базы данных" line.
type Caption struct {
	Kind         CaptionKind
	Word         string // as written: "Рисунок", "Рис.", "Таблица", "Табл."
	Prefix       string // appendix letter in "Рисунок А.2"
	Number       string // "3" or "2.1"
	Sep          string // "—", "–", "-", ".", ":" or "" when nothing follows the number
	Title        string
	Continuation bool // "Продолжение таблицы 2"
}

var (
	captionRe      = regexp.MustCompile(`^(?i)(рисунок|рис\.?|таблица|табл\.?)\s*№?\s*(?:([А-ЯЁA-Z])\.)?(\d+(?:\.\d+)?)(.*)$`)
	continuationRe = regexp.MustCompile(`^(?i)продолжение\s+табл(?:ицы|\.)?\s*(?:([А-ЯЁA-Z])\.)?(\d+(?:\.\d+)?)`)
)

// ParseCaption recognises figure captions and table titles.
func ParseCaption(text string) (Caption, bool) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > 300 {
		return Caption{}, false
	}
	if m := continuationRe.FindStringSubmatch(text); m != nil {
		return Caption{Kind: CaptionTable, Prefix: m[1], Number: m[2], Continuation: true}, true
	}
	m := captionRe.FindStringSubmatch(text)
	if m == nil {
		return Caption{}, false
	}
	c := Caption{Word: m[1], Prefix: m[2], Number: m[3], Kind: CaptionFigure}
	if strings.HasPrefix(strings.ToLower(m[1]), "т") {
		c.Kind = CaptionTable
	}
	rest := strings.TrimSpace(m[4])
	if r, size := utf8.DecodeRuneInString(rest); strings.ContainsRune("—–-.:", r) && size > 0 {
		c.Sep = string(r)
		rest = strings.TrimSpace(rest[size:])
	} else if rest != "" && !unicode.IsSpace(rune(m[4][0])) {
		// "Рисунок 12а" is not a caption number we understand.
		return Caption{}, false
	}
	c.Title = rest
	return c, true
}

// FullNumber is the number with its appendix prefix: "А.2" or "3".
func (c Caption) FullNumber() string {
	if c.Prefix != "" {
		return c.Prefix + "." + c.Number
	}
	return c.Number
}

// Canonical reports whether the caption follows ГОСТ 7.32-2017 6.5.7 and
// 6.6.3: full word, a dash, a title starting with a capital letter and no
// period at the end. Both the em dash used in the standard and the en dash
// many university guides use are accepted.
func (c Caption) Canonical() bool {
	if c.Continuation {
		return true
	}
	word := map[CaptionKind]string{CaptionFigure: "Рисунок", CaptionTable: "Таблица"}[c.Kind]
	if c.Word != word || (c.Sep != "—" && c.Sep != "–") || c.Title == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(c.Title)
	return !unicode.IsLower(first) && !strings.HasSuffix(c.Title, ".")
}
