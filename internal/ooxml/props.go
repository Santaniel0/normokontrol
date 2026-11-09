package ooxml

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// Namespaces of WordprocessingML. Strict OOXML is accepted for reading.
const (
	NSWordMain   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsWordStrict = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	nsDrawing    = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsMath       = "http://schemas.openxmlformats.org/officeDocument/2006/math"
	nsMarkup     = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsPkgRels    = "http://schemas.openxmlformats.org/package/2006/relationships"
)

func isW(n xml.Name) bool { return n.Space == NSWordMain || n.Space == nsWordStrict }

// attr returns a w: attribute by local name; unqualified attributes are
// accepted too because some generators omit the prefix.
func attr(se xml.StartElement, local string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == local && (a.Name.Space == "" || isW(a.Name)) {
			return a.Value, true
		}
	}
	return "", false
}

// onOff reads ST_OnOff: an absent w:val means true.
func onOff(se xml.StartElement) bool {
	v, ok := attr(se, "val")
	if !ok {
		return true
	}
	switch v {
	case "0", "false", "off", "none":
		return false
	}
	return true
}

var unitTwips = map[string]float64{"mm": 1440 / 25.4, "cm": 1440 / 2.54, "in": 1440, "pt": 20, "pc": 240, "pi": 240}

// parseTwips accepts plain twips and the "universal measure" form ("30mm").
func parseTwips(v string) (format.Twips, error) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		return format.Twips(n), nil
	}
	for suf, k := range unitTwips {
		if num, ok := strings.CutSuffix(v, suf); ok {
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				break
			}
			return format.Twips(math.Round(f * k)), nil
		}
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return format.Twips(math.Round(f)), nil
	}
	return 0, fmt.Errorf("bad measure %q", v)
}

func twipsAttr(se xml.StartElement, local string) (format.Opt[format.Twips], error) {
	v, ok := attr(se, local)
	if !ok {
		return format.Opt[format.Twips]{}, nil
	}
	t, err := parseTwips(v)
	if err != nil {
		return format.Opt[format.Twips]{}, fmt.Errorf("%s/@%s: %w", se.Name.Local, local, err)
	}
	return format.Some(t), nil
}

// readParaProps consumes a w:pPr element (the start tag is already read).
// The paragraph style and a nested w:sectPr are returned separately: the
// style is not a cascaded property and the sectPr closes a section.
func readParaProps(d *xml.Decoder) (p format.ParaProps, style format.StyleID, sect *Section, err error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return p, style, sect, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return p, style, sect, nil
		case xml.StartElement:
			if !isW(t.Name) {
				if err := d.Skip(); err != nil {
					return p, style, sect, err
				}
				continue
			}
			switch t.Name.Local {
			case "pStyle":
				v, _ := attr(t, "val")
				style = format.StyleID(v)
			case "jc":
				if v, ok := attr(t, "val"); ok {
					if a, ok := format.ParseJc(v); ok {
						p.Align = format.Some(a)
					}
				}
			case "ind":
				if err := readInd(t, &p); err != nil {
					return p, style, sect, err
				}
			case "spacing":
				if err := readSpacing(t, &p); err != nil {
					return p, style, sect, err
				}
			case "outlineLvl":
				if v, ok := attr(t, "val"); ok {
					if n, err := strconv.Atoi(v); err == nil {
						if n > 8 {
							n = -1 // 9 means "body text" in Word
						}
						p.OutlineLevel = format.Some(n)
					}
				}
			case "pageBreakBefore":
				p.PageBreakBefore = format.Some(onOff(t))
			case "numPr":
				n, err := readNumPr(d)
				if err != nil {
					return p, style, sect, err
				}
				p.Numbered = format.Some(n)
				continue
			case "sectPr":
				s, err := readSectPr(d)
				if err != nil {
					return p, style, sect, err
				}
				sect = &s
				continue
			}
			if err := d.Skip(); err != nil {
				return p, style, sect, err
			}
		}
	}
}

func readInd(se xml.StartElement, p *format.ParaProps) error {
	var err error
	for _, k := range []string{"left", "start"} {
		if o, e := twipsAttr(se, k); e != nil {
			err = e
		} else if o.Set {
			p.IndLeft = o
		}
	}
	for _, k := range []string{"right", "end"} {
		if o, e := twipsAttr(se, k); e != nil {
			err = e
		} else if o.Set {
			p.IndRight = o
		}
	}
	if o, e := twipsAttr(se, "firstLine"); e != nil {
		err = e
	} else if o.Set {
		p.FirstLine = o
	}
	// hanging and firstLine are mutually exclusive; hanging wins in Word.
	if o, e := twipsAttr(se, "hanging"); e != nil {
		err = e
	} else if o.Set {
		p.FirstLine = format.Some(-o.Val)
	}
	return err
}

func readSpacing(se xml.StartElement, p *format.ParaProps) error {
	before, err := twipsAttr(se, "before")
	if err != nil {
		return err
	}
	after, err := twipsAttr(se, "after")
	if err != nil {
		return err
	}
	if before.Set {
		p.Before = before
	}
	if after.Set {
		p.After = after
	}
	v, ok := attr(se, "line")
	if !ok {
		return nil
	}
	n, err := parseTwips(v)
	if err != nil {
		return fmt.Errorf("spacing/@line: %w", err)
	}
	rule := format.LineAuto
	if r, ok := attr(se, "lineRule"); ok {
		switch r {
		case "exact":
			rule = format.LineExact
		case "atLeast":
			rule = format.LineAtLeast
		}
	}
	p.Line = format.Some(format.LineSpacing{Value: int(n), Rule: rule})
	return nil
}

// readNumPr reports whether numbering is switched on; numId 0 removes it.
func readNumPr(d *xml.Decoder) (bool, error) {
	numbered := false
	for {
		tok, err := d.Token()
		if err != nil {
			return false, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return numbered, nil
		case xml.StartElement:
			if isW(t.Name) && t.Name.Local == "numId" {
				v, _ := attr(t, "val")
				numbered = v != "" && v != "0"
			}
			if err := d.Skip(); err != nil {
				return false, err
			}
		}
	}
}

// readRunProps consumes a w:rPr element and returns the run style separately.
func readRunProps(d *xml.Decoder) (format.RunProps, format.StyleID, error) {
	var r format.RunProps
	var style format.StyleID
	for {
		tok, err := d.Token()
		if err != nil {
			return r, style, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return r, style, nil
		case xml.StartElement:
			if isW(t.Name) {
				switch t.Name.Local {
				case "rStyle":
					v, _ := attr(t, "val")
					style = format.StyleID(v)
				case "rFonts":
					if f, ok := fontRef(t, "ascii"); ok {
						r.FontASCII = format.Some(f)
					}
					if f, ok := fontRef(t, "hAnsi"); ok {
						r.FontHAnsi = format.Some(f)
					}
				case "sz":
					if v, ok := attr(t, "val"); ok {
						if n, err := strconv.ParseFloat(v, 64); err == nil {
							r.Size = format.Some(format.HalfPoints(math.Round(n)))
						}
					}
				case "b":
					r.Bold = format.Some(onOff(t))
				case "i":
					r.Italic = format.Some(onOff(t))
				case "caps":
					r.Caps = format.Some(onOff(t))
				case "color":
					if v, ok := attr(t, "val"); ok {
						r.Color = format.Some(strings.ToUpper(v))
					}
				}
			}
			if err := d.Skip(); err != nil {
				return r, style, err
			}
		}
	}
}

func fontRef(se xml.StartElement, slot string) (format.FontRef, bool) {
	var f format.FontRef
	name, hasName := attr(se, slot)
	if hasName {
		f.Name = name
	}
	if th, ok := attr(se, slot+"Theme"); ok {
		if tf, ok := format.ParseThemeFont(th); ok {
			f.Theme = tf
			return f, true
		}
	}
	return f, hasName
}
