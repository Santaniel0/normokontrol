package rules

import (
	"strings"
	"unicode"

	"github.com/Santaniel0/normokontrol/internal/format"
	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

// structuralNames are headings that ГОСТ 7.32-2017 (6.2.1) treats as
// structural elements: capitals, centred, no period.
func structuralNames(p *profile.Profile) map[string]bool {
	set := map[string]bool{"СОДЕРЖАНИЕ": true, "ОГЛАВЛЕНИЕ": true, "АННОТАЦИЯ": true}
	for _, list := range [][]string{p.Structure.Order, p.Structure.Required, p.Structure.Bibliography} {
		for _, n := range list {
			set[model.Normalize(n)] = true
		}
	}
	return set
}

func checkHeadings(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	structural := structuralNames(p)
	for _, para := range d.Paras {
		if para.Role != model.RoleHeading {
			continue
		}
		text := strings.TrimSpace(para.Text)
		norm := model.Normalize(text)
		isStruct := structural[norm] || strings.HasPrefix(norm, "ПРИЛОЖЕНИЕ")
		add := func(sev Severity, m, detail string) {
			out = append(out, finding(HeadingFormat, sev, para.Index, m, detail))
		}
		if strings.HasSuffix(text, ".") && !strings.HasSuffix(text, "...") {
			add(SeverityError, msg.HeadingPeriod, "")
		}
		if structural[norm] {
			if hasLower(text) && !allCaps(para) {
				add(SeverityError, msg.StructuralCase, "")
			}
			if para.Format.Align.Val != format.AlignCenter {
				add(SeverityError, msg.StructuralCenter, msg.Origin(para.Format.Align.From))
			}
		}
		if !isStruct && startsLower(text) {
			add(SeverityError, msg.HeadingLowercase, "")
		}
		if !para.Styled {
			add(SeverityWarning, msg.HeadingNoStyle, "")
		}
	}
	return out
}

func hasLower(s string) bool {
	return strings.IndexFunc(s, unicode.IsLower) >= 0
}

// allCaps covers headings typed in lower case but displayed in capitals
// through the "Все прописные" font option.
func allCaps(p *model.Paragraph) bool {
	return p.Src != nil && len(p.Src.Runs) > 0 && p.Src.Runs[0].Props.Caps.Val
}

func startsLower(s string) bool {
	s = strings.TrimLeft(s, "0123456789. \t")
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

func checkStructure(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	order := map[string]int{}
	for i, n := range p.Structure.Order {
		order[model.Normalize(n)] = i
	}
	present := map[string]bool{}
	bibNames := map[string]bool{}
	for _, n := range p.Structure.Bibliography {
		bibNames[model.Normalize(n)] = true
	}
	knownBib := map[string]bool{"СПИСОК ЛИТЕРАТУРЫ": true, "СПИСОК ИСПОЛЬЗОВАННОЙ ЛИТЕРАТУРЫ": true,
		"БИБЛИОГРАФИЧЕСКИЙ СПИСОК": true, "СПИСОК ИСТОЧНИКОВ": true, "СПИСОК ИСПОЛЬЗОВАННЫХ ИСТОЧНИКОВ": true}
	maxSeen, maxName := -1, ""
	for _, para := range d.Paras {
		if para.Role != model.RoleHeading {
			continue
		}
		norm := model.Normalize(para.Text)
		present[norm] = true
		if knownBib[norm] && len(bibNames) > 0 && !bibNames[norm] {
			out = append(out, finding(StructureList, SeverityWarning, para.Index,
				msg.BibHeading(strings.TrimSpace(para.Text), p.Structure.Bibliography[0]), ""))
		}
		pos, ok := order[norm]
		if !ok {
			continue
		}
		if pos < maxSeen {
			out = append(out, finding(StructureList, SeverityError, para.Index, msg.StructureOrder(norm, maxName), ""))
			continue
		}
		maxSeen, maxName = pos, norm
	}
	for _, n := range p.Structure.Required {
		if !present[model.Normalize(n)] {
			out = append(out, finding(StructureList, SeverityError, -1, msg.StructureMissing(n), ""))
		}
	}
	return out
}
