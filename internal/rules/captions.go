package rules

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

func checkCaptions(d *model.Document, p *profile.Profile) []Finding {
	out := missingCaptions(d)
	refs := mentions(d)
	for _, kind := range []model.CaptionKind{model.CaptionFigure, model.CaptionTable} {
		table := kind == model.CaptionTable
		var caps []*model.Paragraph
		for _, para := range d.Paras {
			if para.Role == model.RoleCaption && para.Caption.Kind == kind && !para.Caption.Continuation {
				caps = append(caps, para)
			}
		}
		for _, c := range caps {
			if !c.Caption.Canonical() {
				out = append(out, finding(Captions, SeverityError, c.Index, msg.CaptionFormat(table), ""))
			}
			if !refs[kind][c.Caption.FullNumber()] {
				out = append(out, finding(Captions, SeverityError, c.Index, msg.NotReferenced(table, c.Caption.FullNumber()), ""))
			}
		}
		out = append(out, numbering(caps, table, p.Captions.Numbering)...)
	}
	return out
}

// missingCaptions finds pictures without a caption after them and tables
// without a "Таблица N" line before them.
func missingCaptions(d *model.Document) []Finding {
	var out []Finding
	prevTable := 0
	for i, para := range d.Paras {
		if para.Region == model.RegionFront {
			prevTable = para.Src.Table
			continue
		}
		switch {
		case para.Role == model.RoleFigure:
			next := nearest(d, i, +1)
			if next != nil && next.Role == model.RoleFigure {
				continue // a group of pictures shares the caption after the last one
			}
			if next == nil || next.Role != model.RoleCaption || next.Caption.Kind != model.CaptionFigure {
				out = append(out, finding(Captions, SeverityError, i, msg.FigureNoCaption, ""))
			}
		case para.Src.Table > 0 && para.Src.Table != prevTable:
			prev := nearest(d, i, -1)
			if prev == nil || prev.Role != model.RoleCaption || prev.Caption.Kind != model.CaptionTable {
				out = append(out, finding(Captions, SeverityError, i, msg.TableNoCaption, ""))
			}
		}
		prevTable = para.Src.Table
	}
	return out
}

// nearest returns the closest non-empty paragraph within two steps.
func nearest(d *model.Document, i, dir int) *model.Paragraph {
	for j, steps := i+dir, 0; j >= 0 && j < len(d.Paras) && steps < 3; j, steps = j+dir, steps+1 {
		if d.Paras[j].Role != model.RoleEmpty {
			return d.Paras[j]
		}
	}
	return nil
}

func numbering(caps []*model.Paragraph, table bool, mode profile.NumberingMode) []Finding {
	var out []Finding
	byPrefix := map[string][]*model.Paragraph{}
	var prefixes []string
	for _, c := range caps {
		pf := c.Caption.Prefix
		if _, ok := byPrefix[pf]; !ok {
			prefixes = append(prefixes, pf)
		}
		byPrefix[pf] = append(byPrefix[pf], c)
	}
	for _, pf := range prefixes {
		group := byPrefix[pf]
		dotted := 0
		for _, c := range group {
			if strings.Contains(c.Caption.Number, ".") {
				dotted++
			}
		}
		bySection := dotted == len(group)
		if pf == "" {
			switch {
			case dotted > 0 && dotted < len(group):
				out = append(out, finding(Captions, SeverityError, group[0].Index, msg.NumberingMixed(table), ""))
				continue
			case mode == profile.NumberingContinuous && bySection:
				out = append(out, finding(Captions, SeverityError, group[0].Index, msg.NumberingScheme(table, false), ""))
				continue
			case mode == profile.NumberingBySection && !bySection:
				out = append(out, finding(Captions, SeverityError, group[0].Index, msg.NumberingScheme(table, true), ""))
				continue
			}
		}
		out = append(out, sequence(group, table, bySection && pf == "")...)
	}
	return out
}

// sequence checks 1, 2, 3 or 1.1, 1.2, 2.1. After a gap the expectation
// resynchronises, so one missing number produces one finding, not a cascade.
func sequence(group []*model.Paragraph, table, bySection bool) []Finding {
	var out []Finding
	prevMajor, prevMinor := 0, 0
	for _, c := range group {
		major, minor := 0, 0
		num := c.Caption.Number
		if bySection {
			a, b, _ := strings.Cut(num, ".")
			major, _ = strconv.Atoi(a)
			minor, _ = strconv.Atoi(b)
		} else {
			minor, _ = strconv.Atoi(num)
		}
		wantMinor := prevMinor + 1
		if bySection && major != prevMajor {
			wantMinor = 1
		}
		if minor != wantMinor || (bySection && major < prevMajor) {
			want := strconv.Itoa(wantMinor)
			if bySection {
				want = strconv.Itoa(max(major, prevMajor)) + "." + want
			}
			if c.Caption.Prefix != "" {
				want = c.Caption.Prefix + "." + want
			}
			out = append(out, finding(Captions, SeverityError, c.Index, msg.NumberingGap(table, c.Caption.FullNumber(), want), ""))
		}
		prevMajor, prevMinor = major, minor
	}
	return out
}

var (
	mentionRe  = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(рис\.|рисун\p{L}*|табл\.|таблиц\p{L}*)\s*(?:№\s*)?((?:[А-ЯЁA-Z]\.)?\d+(?:\.\d+)?(?:\s*(?:,|и|–|—|-)\s*(?:[А-ЯЁA-Z]\.)?\d+(?:\.\d+)?)*)`)
	refTokenRe = regexp.MustCompile(`(?:[А-ЯЁA-Zа-яёa-z]\.)?\d+(?:\.\d+)?|[–—-]`)
)

// mentions collects "рисунок 3", "рис. 2.1", "таблицы 1 и 2", "рисунки 2–4"
// from everything that is not itself a caption, heading or list of sources.
func mentions(d *model.Document) map[model.CaptionKind]map[string]bool {
	out := map[model.CaptionKind]map[string]bool{model.CaptionFigure: {}, model.CaptionTable: {}}
	for _, p := range d.Paras {
		switch p.Role {
		case model.RoleBody, model.RoleListItem, model.RoleTableCell, model.RoleFormula, model.RoleCode:
		default:
			continue
		}
		for _, m := range mentionRe.FindAllStringSubmatch(p.Text, -1) {
			kind := model.CaptionFigure
			if strings.HasPrefix(strings.ToLower(m[1]), "т") {
				kind = model.CaptionTable
			}
			for _, n := range expandRefs(m[2]) {
				out[kind][n] = true
			}
		}
	}
	return out
}

// expandRefs turns "2–4, 7" into 2, 3, 4, 7 and "1.2–1.4" into 1.2, 1.3, 1.4.
func expandRefs(s string) []string {
	toks := refTokenRe.FindAllString(s, -1)
	var out []string
	for i := 0; i < len(toks); i++ {
		t := upperPrefix(toks[i])
		if t == "-" || t == "–" || t == "—" {
			continue
		}
		out = append(out, t)
		if i+2 < len(toks) && strings.ContainsAny(toks[i+1], "-–—") {
			end := upperPrefix(toks[i+2])
			out = append(out, between(t, end)...)
			out = append(out, end)
			i += 2
		}
	}
	return out
}

func upperPrefix(t string) string {
	if len(t) > 1 && strings.Contains(t, ".") && !strings.ContainsAny(t[:1], "0123456789") {
		return strings.ToUpper(t[:strings.Index(t, ".")]) + t[strings.Index(t, "."):]
	}
	return t
}

// between returns the numbers strictly between a and b when they share
// everything but the last component; ranges longer than 50 are ignored.
func between(a, b string) []string {
	ia, ib := strings.LastIndex(a, "."), strings.LastIndex(b, ".")
	if a[:ia+1] != b[:ib+1] {
		return nil
	}
	x, err1 := strconv.Atoi(a[ia+1:])
	y, err2 := strconv.Atoi(b[ib+1:])
	if err1 != nil || err2 != nil || y <= x || y-x > 50 {
		return nil
	}
	var out []string
	for n := x + 1; n < y; n++ {
		out = append(out, a[:ia+1]+strconv.Itoa(n))
	}
	return out
}
