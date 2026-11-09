package rules

import (
	"math"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/format"
	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

func checkPage(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	tol := float64(p.Page.ToleranceMM)
	off := func(got format.Twips, want profile.Millimeters) bool {
		return want > 0 && math.Abs(got.Millimeters()-float64(want)) > tol
	}
	for i, s := range d.Sections {
		anchor := d.SectionStart[i]
		prefix := msg.SectionPrefix(i, len(d.Sections))
		if s.HasPageSize && p.Page.WidthMM > 0 && p.Page.HeightMM > 0 {
			w, h := s.PageWidth, s.PageHeight
			if s.Landscape {
				w, h = h, w
			}
			if off(w, p.Page.WidthMM) || off(h, p.Page.HeightMM) {
				out = append(out, finding(PageMargins, SeverityError, anchor,
					prefix+msg.PageSize(round(w.Millimeters()), round(h.Millimeters()), float64(p.Page.WidthMM), float64(p.Page.HeightMM)), ""))
			}
		}
		// Landscape pages for wide tables keep their margins relative to the
		// rotated sheet, and universities differ on what they expect there.
		if !s.HasMargins || s.Landscape {
			continue
		}
		sides := []struct {
			side msg.Side
			got  format.Twips
			want profile.Millimeters
		}{
			{msg.SideLeft, s.MarginLeft, p.Page.MarginLeftMM},
			{msg.SideRight, s.MarginRight, p.Page.MarginRightMM},
			{msg.SideTop, abs(s.MarginTop), p.Page.MarginTopMM},
			{msg.SideBottom, abs(s.MarginBottom), p.Page.MarginBottomMM},
		}
		for _, sd := range sides {
			if off(sd.got, sd.want) {
				out = append(out, finding(PageMargins, SeverityError, anchor,
					prefix+msg.Margin(sd.side, round(sd.got.Millimeters()), float64(sd.want)), ""))
			}
		}
	}
	return out
}

func abs(t format.Twips) format.Twips {
	if t < 0 {
		return -t
	}
	return t
}

func round(f float64) float64 { return math.Round(f*10) / 10 }

// textRoles are paragraphs whose font and spacing follow the body text rules.
var textRoles = []model.Role{model.RoleBody, model.RoleListItem, model.RoleBibEntry}

func checkFont(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	paras := mainText(d, textRoles...)
	var names []string
	var sizes []format.HalfPoints
	for _, para := range paras {
		names = append(names, para.Font.Name.Val)
		sizes = append(sizes, para.Font.Size.Val)
	}
	domName, nameOK := dominant(names)
	domSize, sizeOK := dominant(sizes)
	want := p.Body.Font
	for _, para := range paras {
		f := para.Font
		switch {
		case f.Name.Val == "":
			// The theme part was missing: the font is unknown, not wrong.
		case want != "" && !strings.EqualFold(f.Name.Val, want):
			sev := SeverityWarning
			if p.Body.FontRequired {
				sev = SeverityError
			}
			out = append(out, finding(BodyFont, sev, para.Index, msg.FontFamily(f.Name.Val, want, p.Body.FontRequired), msg.Origin(f.Name.From)))
		case want == "" && nameOK && f.Name.Val != domName:
			out = append(out, finding(BodyFont, SeverityWarning, para.Index, msg.FontMixed(f.Name.Val, domName), msg.Origin(f.Name.From)))
		}
		for _, other := range para.Fonts[min(1, len(para.Fonts)):] {
			if other.Chars >= 5 && other.Name.Val != "" && !strings.EqualFold(other.Name.Val, f.Name.Val) {
				out = append(out, finding(BodyFont, SeverityWarning, para.Index, msg.FontFragment(other.Name.Val, other.Chars), msg.Origin(other.Name.From)))
				break
			}
		}
		pt := f.Size.Val.Points()
		switch {
		case p.Body.SizePt > 0 && math.Abs(pt-float64(p.Body.SizePt)) > 0.01:
			out = append(out, finding(BodyFont, SeverityError, para.Index, msg.FontSize(pt, float64(p.Body.SizePt)), msg.Origin(f.Size.From)))
		case p.Body.MinSizePt > 0 && pt < float64(p.Body.MinSizePt):
			out = append(out, finding(BodyFont, SeverityError, para.Index, msg.FontSizeMin(pt, float64(p.Body.MinSizePt)), msg.Origin(f.Size.From)))
		case p.Body.SizePt == 0 && sizeOK && f.Size.Val != domSize:
			out = append(out, finding(BodyFont, SeverityWarning, para.Index, msg.FontSizeMixed(pt, domSize.Points()), msg.Origin(f.Size.From)))
		}
	}
	return out
}

func checkSpacing(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	paras := mainText(d, textRoles...)
	var vals []format.LineSpacing
	for _, para := range paras {
		vals = append(vals, para.Format.Line.Val)
	}
	dom, domOK := dominant(vals)
	for _, para := range paras {
		l := para.Format.Line
		if p.Body.LineSpacing > 0 {
			m, ok := l.Val.Multiple()
			if !ok || math.Abs(m-p.Body.LineSpacing) > 0.01 {
				out = append(out, finding(BodySpacing, SeverityError, para.Index, msg.Spacing(l.Val, p.Body.LineSpacing), msg.Origin(l.From)))
			}
		} else if domOK && l.Val != dom {
			out = append(out, finding(BodySpacing, SeverityWarning, para.Index, msg.SpacingMixed(l.Val, dom), msg.Origin(l.From)))
		}
	}
	return out
}

func checkAlign(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	paras := mainText(d, model.RoleBody)
	var vals []format.Alignment
	for _, para := range paras {
		vals = append(vals, para.Format.Align.Val)
	}
	dom, domOK := dominant(vals)
	for _, para := range paras {
		a := para.Format.Align
		switch {
		case p.Body.Align != 0 && a.Val != p.Body.Align:
			out = append(out, finding(BodyAlign, SeverityError, para.Index, msg.Align(a.Val, p.Body.Align), msg.Origin(a.From)))
		case p.Body.Align == 0 && domOK && a.Val != dom:
			out = append(out, finding(BodyAlign, SeverityWarning, para.Index, msg.AlignMixed(a.Val, dom), msg.Origin(a.From)))
		}
	}
	return out
}

func checkIndent(d *model.Document, p *profile.Profile) []Finding {
	var out []Finding
	paras := mainText(d, model.RoleBody)
	var vals []format.Twips
	for _, para := range paras {
		vals = append(vals, para.Format.FirstLine.Val)
	}
	dom, domOK := dominant(vals)
	want := float64(p.Body.FirstLineIndentCM)
	tol := math.Max(float64(p.Body.IndentToleranceCM), 0.005)
	for _, para := range paras {
		if strings.HasPrefix(para.Text, "\t") || strings.HasPrefix(para.Text, "  ") {
			out = append(out, finding(BodyIndent, SeverityError, para.Index, msg.IndentFake(strings.HasPrefix(para.Text, "\t")), ""))
			continue
		}
		fl := para.Format.FirstLine
		got := fl.Val.Centimeters()
		switch {
		case want > 0 && math.Abs(got-want) > tol:
			out = append(out, finding(BodyIndent, SeverityError, para.Index, msg.Indent(round2(got), want), msg.Origin(fl.From)))
		case want == 0 && domOK && fl.Val != dom:
			out = append(out, finding(BodyIndent, SeverityWarning, para.Index, msg.IndentMixed(round2(got), round2(dom.Centimeters())), msg.Origin(fl.From)))
		}
	}
	return out
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// checkEmpty reports runs of empty paragraphs in the main text. A single
// empty line next to a picture, table or formula is tolerated: many
// university guides ask for exactly that.
func checkEmpty(d *model.Document, _ *profile.Profile) []Finding {
	var out []Finding
	for i := 0; i < len(d.Paras); i++ {
		p := d.Paras[i]
		if p.Role != model.RoleEmpty || p.Region != model.RegionMain || p.Src.PageBreak {
			continue
		}
		j := i
		for j < len(d.Paras) && d.Paras[j].Role == model.RoleEmpty && !d.Paras[j].Src.PageBreak {
			j++
		}
		n := j - i
		prev, next := role(d, i-1), role(d, j)
		nearObject := isObject(prev) || isObject(next)
		atEnd := j == len(d.Paras)
		if !atEnd && !(n == 1 && nearObject) {
			sev := SeverityWarning
			if n >= 2 {
				sev = SeverityError
			}
			out = append(out, finding(BodyEmpty, sev, i, msg.EmptyParagraphs(n, next == model.RoleHeading), ""))
		}
		i = j - 1
	}
	return out
}

func role(d *model.Document, i int) model.Role {
	if i < 0 || i >= len(d.Paras) {
		return 0
	}
	return d.Paras[i].Role
}

func isObject(r model.Role) bool {
	switch r {
	case model.RoleFigure, model.RoleCaption, model.RoleTableCell, model.RoleFormula, model.RoleCode:
		return true
	}
	return false
}
