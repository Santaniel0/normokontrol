// Package rules checks a classified document against a profile.
package rules

import (
	"context"
	"fmt"
	"sort"

	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

// ID identifies a rule in profiles and reports.
type ID string

// Rule ids. They are part of the profile format and the JSON schema.
const (
	PageMargins   ID = "page.margins"
	BodyFont      ID = "body.font"
	BodySpacing   ID = "body.spacing"
	BodyAlign     ID = "body.align"
	BodyIndent    ID = "body.indent"
	BodyEmpty     ID = "body.empty"
	HeadingFormat ID = "heading.format"
	StructureList ID = "structure"
	Captions      ID = "captions"
	Citations     ID = "refs.citations"
	CitationOrder ID = "refs.order"
	Homoglyphs    ID = "text.homoglyph"
)

// Severity of a finding.
type Severity int

const (
	SeverityError Severity = iota + 1
	SeverityWarning
)

func (s Severity) String() string {
	if s == SeverityWarning {
		return "warning"
	}
	return "error"
}

// Finding is one problem. Para is the paragraph the problem is attached to;
// -1 means the document as a whole.
type Finding struct {
	Rule     ID
	Severity Severity
	Para     int
	Message  string // identical messages are grouped in the text report
	Detail   string // per-instance explanation, such as where a value comes from
}

// Rule is one check.
type Rule interface {
	ID() ID
	Check(d *model.Document, p *profile.Profile) []Finding
}

type rule struct {
	id    ID
	check func(*model.Document, *profile.Profile) []Finding
}

func (r rule) ID() ID { return r.id }

func (r rule) Check(d *model.Document, p *profile.Profile) []Finding { return r.check(d, p) }

// All returns every rule in report order.
func All() []Rule {
	return []Rule{
		rule{PageMargins, checkPage},
		rule{StructureList, checkStructure},
		rule{HeadingFormat, checkHeadings},
		rule{BodyFont, checkFont},
		rule{BodySpacing, checkSpacing},
		rule{BodyAlign, checkAlign},
		rule{BodyIndent, checkIndent},
		rule{BodyEmpty, checkEmpty},
		rule{Captions, checkCaptions},
		rule{Citations, checkCitations},
		rule{CitationOrder, checkCitationOrder},
		rule{Homoglyphs, checkHomoglyphs},
	}
}

// UnknownRuleError reports a rule id in a profile that does not exist.
type UnknownRuleError struct{ ID string }

func (e *UnknownRuleError) Error() string { return fmt.Sprintf("unknown rule %q", e.ID) }

// Enabled returns the rules the profile does not disable.
func Enabled(p *profile.Profile) ([]Rule, error) {
	off := map[ID]bool{}
	known := map[ID]bool{}
	for _, r := range All() {
		known[r.ID()] = true
	}
	for _, s := range p.Rules.Disable {
		if !known[ID(s)] {
			return nil, &UnknownRuleError{ID: s}
		}
		off[ID(s)] = true
	}
	var out []Rule
	for _, r := range All() {
		if !off[r.ID()] {
			out = append(out, r)
		}
	}
	return out, nil
}

// Run applies the enabled rules and orders findings by position.
func Run(ctx context.Context, d *model.Document, p *profile.Profile) ([]Finding, error) {
	enabled, err := Enabled(p)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, r := range enabled {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, r.Check(d, p)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Para < out[j].Para })
	return out, nil
}

// mainText selects paragraphs of the main body with the given roles.
// Appendices are left out: they hold listings and tables with their own rules.
func mainText(d *model.Document, roles ...model.Role) []*model.Paragraph {
	var out []*model.Paragraph
	for _, p := range d.Paras {
		if p.Region != model.RegionMain {
			continue
		}
		for _, r := range roles {
			if p.Role == r {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// dominant returns the most common value when it covers at least half of
// three or more samples; otherwise the document has no clear norm to compare
// against and consistency checks stay quiet.
func dominant[T comparable](vals []T) (T, bool) {
	var zero T
	if len(vals) < 3 {
		return zero, false
	}
	counts := map[T]int{}
	var best T
	for _, v := range vals {
		counts[v]++
		if counts[v] > counts[best] {
			best = v
		}
	}
	if counts[best]*2 < len(vals) {
		return zero, false
	}
	return best, true
}

func finding(id ID, sev Severity, para int, message, detail string) Finding {
	return Finding{Rule: id, Severity: sev, Para: para, Message: message, Detail: detail}
}
