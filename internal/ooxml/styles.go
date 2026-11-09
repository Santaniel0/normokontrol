package ooxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// ParseStyles parses styles.xml.
func ParseStyles(data []byte) (format.StyleSheet, error) {
	sheet := format.StyleSheet{Styles: make(map[format.StyleID]format.Style)}
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return sheet, nil
		}
		if err != nil {
			return sheet, fmt.Errorf("styles.xml: %w: %w", ErrMalformedXML, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !isW(se.Name) {
			continue
		}
		switch se.Name.Local {
		case "rPrDefault", "pPrDefault", "docDefaults", "styles":
			// containers: descend
		case "rPr":
			// Only reached for w:rPrDefault/w:rPr: style children are read below.
			rp, _, err := readRunProps(d)
			if err != nil {
				return sheet, fmt.Errorf("styles.xml: %w", err)
			}
			sheet.DefaultRun = rp
		case "pPr":
			pp, _, _, err := readParaProps(d)
			if err != nil {
				return sheet, fmt.Errorf("styles.xml: %w", err)
			}
			sheet.DefaultPara = pp
		case "style":
			st, err := readStyle(d, se)
			if err != nil {
				return sheet, fmt.Errorf("styles.xml: style %q: %w", st.ID, err)
			}
			sheet.Styles[st.ID] = st
		default:
			if err := d.Skip(); err != nil {
				return sheet, fmt.Errorf("styles.xml: %w", err)
			}
		}
	}
}

func readStyle(d *xml.Decoder, se xml.StartElement) (format.Style, error) {
	var st format.Style
	id, _ := attr(se, "styleId")
	st.ID = format.StyleID(id)
	switch v, _ := attr(se, "type"); v {
	case "paragraph", "":
		st.Type = format.StyleParagraph
	case "character":
		st.Type = format.StyleCharacter
	case "table":
		st.Type = format.StyleTable
	case "numbering":
		st.Type = format.StyleNumbering
	}
	if v, ok := attr(se, "default"); ok {
		st.Default = v == "1" || v == "true" || v == "on"
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return st, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return st, nil
		case xml.StartElement:
			if !isW(t.Name) {
				if err := d.Skip(); err != nil {
					return st, err
				}
				continue
			}
			switch t.Name.Local {
			case "pPr":
				if st.Para, _, _, err = readParaProps(d); err != nil {
					return st, err
				}
				continue
			case "rPr":
				if st.Run, _, err = readRunProps(d); err != nil {
					return st, err
				}
				continue
			case "name":
				st.Name, _ = attr(t, "val")
			case "basedOn":
				v, _ := attr(t, "val")
				st.BasedOn = format.StyleID(v)
			}
			if err := d.Skip(); err != nil {
				return st, err
			}
		}
	}
}

// ParseTheme reads the Latin typefaces of the theme font scheme.
func ParseTheme(data []byte) (format.Theme, error) {
	var th format.Theme
	d := xml.NewDecoder(bytes.NewReader(data))
	var slot *string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return th, nil
		}
		if err != nil {
			return th, fmt.Errorf("theme: %w: %w", ErrMalformedXML, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != nsDrawing {
				continue
			}
			switch t.Name.Local {
			case "majorFont":
				slot = &th.Major
			case "minorFont":
				slot = &th.Minor
			case "latin":
				if slot != nil {
					for _, a := range t.Attr {
						if a.Name.Local == "typeface" {
							*slot = a.Value
						}
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "majorFont" || t.Name.Local == "minorFont" {
				slot = nil
			}
		}
	}
}
