package ooxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// Section is a w:sectPr: page geometry for the paragraphs before it.
type Section struct {
	PageWidth, PageHeight   format.Twips
	MarginTop, MarginBottom format.Twips
	MarginLeft, MarginRight format.Twips
	HasPageSize, HasMargins bool
	Landscape               bool
}

// Span locates a paragraph inside the raw document.xml so that comments can
// be spliced in without re-encoding the XML.
type Span struct {
	Start       int64 // first byte of the <w:p> start tag
	AnchorStart int64 // after the start tag, or after </w:pPr> when present
	AnchorEnd   int64 // first byte of </w:p>
	End         int64 // after </w:p>
	SelfClosing bool  // written as <w:p/>; AnchorStart == AnchorEnd == End
}

// Run is a w:r with its text. Field codes and deleted text are dropped.
type Run struct {
	StyleID format.StyleID
	Props   format.RunProps
	Text    string
}

// Paragraph is a w:p from the main document body, including table cells.
type Paragraph struct {
	StyleID   format.StyleID
	Props     format.ParaProps
	Runs      []Run
	Drawing   bool // image, shape, chart or embedded object
	Math      bool
	PageBreak bool // contains <w:br w:type="page"/>
	Table     int  // 0 outside tables, otherwise the 1-based number of the outermost table
	Section   int  // index into Body.Sections
	Pos       Span
}

// Text concatenates run texts.
func (p *Paragraph) Text() string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// Body is the parsed w:body.
type Body struct {
	Paragraphs []*Paragraph
	Sections   []Section
	Tables     int
}

// ErrMalformedXML wraps XML syntax errors in document parts.
var ErrMalformedXML = errors.New("malformed XML")

type docParser struct {
	d          *xml.Decoder
	body       *Body
	para       *Paragraph
	run        *Run
	tableDepth int
}

// ParseDocument parses document.xml in one streaming pass.
func ParseDocument(data []byte) (*Body, error) {
	p := &docParser{d: xml.NewDecoder(bytes.NewReader(data)), body: &Body{}}
	if err := p.parse(); err != nil {
		var se *xml.SyntaxError
		if errors.As(err, &se) {
			return nil, fmt.Errorf("document.xml: %w: %w", ErrMalformedXML, err)
		}
		return nil, fmt.Errorf("document.xml: %w", err)
	}
	need := 1
	if n := len(p.body.Paragraphs); n > 0 {
		need = p.body.Paragraphs[n-1].Section + 1
	}
	// A body without a final sectPr is invalid, but Word opens it anyway.
	for len(p.body.Sections) < need {
		p.body.Sections = append(p.body.Sections, Section{})
	}
	return p.body, nil
}

func (p *docParser) parse() error {
	for {
		off := p.d.InputOffset()
		tok, err := p.d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := p.start(t, off); err != nil {
				return err
			}
		case xml.EndElement:
			p.end(t, off)
		case xml.CharData:
			// Text only matters inside w:t, which start() reads directly.
		}
	}
}

func (p *docParser) start(t xml.StartElement, off int64) error {
	if t.Name.Space == nsMath && (t.Name.Local == "oMath" || t.Name.Local == "oMathPara") {
		if p.para != nil {
			p.para.Math = true
		}
		return p.d.Skip()
	}
	if t.Name.Space == nsMarkup && t.Name.Local == "AlternateContent" {
		// Inside runs this is a shape or text box with a fallback copy of the
		// same content; reading both would duplicate the text.
		if p.para != nil {
			p.para.Drawing = true
		}
		return p.d.Skip()
	}
	if !isW(t.Name) {
		return p.d.Skip()
	}
	switch t.Name.Local {
	case "document", "body", "sdt", "sdtContent", "customXml", "smartTag", "hyperlink",
		"ins", "fldSimple", "tr", "tc", "dir", "bdo":
		return nil
	case "tbl":
		if p.tableDepth == 0 {
			p.body.Tables++
		}
		p.tableDepth++
		return nil
	case "p":
		p.para = &Paragraph{Section: len(p.body.Sections), Pos: Span{Start: off, AnchorStart: p.d.InputOffset()}}
		if p.tableDepth > 0 {
			p.para.Table = p.body.Tables
		}
		return nil
	case "pPr":
		if p.para == nil {
			return p.d.Skip()
		}
		props, style, sect, err := readParaProps(p.d)
		if err != nil {
			return err
		}
		p.para.Props, p.para.StyleID = props, style
		p.para.Pos.AnchorStart = p.d.InputOffset()
		if sect != nil {
			p.body.Sections = append(p.body.Sections, *sect)
		}
		return nil
	case "r":
		if p.para != nil {
			p.run = &Run{}
		}
		return nil
	case "rPr":
		if p.run == nil {
			return p.d.Skip()
		}
		props, style, err := readRunProps(p.d)
		p.run.Props, p.run.StyleID = props, style
		return err
	case "t":
		var s string
		if err := p.d.DecodeElement(&s, &t); err != nil {
			return err
		}
		if p.run != nil {
			p.run.Text += s
		}
		return nil
	case "tab", "ptab":
		p.appendText("\t")
	case "br", "cr":
		if v, _ := attr(t, "type"); v == "page" {
			if p.para != nil {
				p.para.PageBreak = true
			}
		} else {
			p.appendText("\n")
		}
	case "noBreakHyphen":
		p.appendText("-")
	case "drawing", "pict", "object":
		if p.para != nil {
			p.para.Drawing = true
		}
	case "sectPr":
		s, err := readSectPr(p.d)
		if err != nil {
			return err
		}
		p.body.Sections = append(p.body.Sections, s)
		return nil
	}
	return p.d.Skip()
}

func (p *docParser) appendText(s string) {
	if p.run != nil {
		p.run.Text += s
	}
}

func (p *docParser) end(t xml.EndElement, off int64) {
	if !isW(t.Name) {
		return
	}
	switch t.Name.Local {
	case "tbl":
		p.tableDepth--
	case "r":
		if p.para != nil && p.run != nil {
			p.para.Runs = append(p.para.Runs, *p.run)
		}
		p.run = nil
	case "p":
		if p.para == nil {
			return
		}
		end := p.d.InputOffset()
		p.para.Pos.AnchorEnd = off
		p.para.Pos.End = end
		// The decoder reports <w:p/> as a start and an end token; the end
		// token consumes no input.
		p.para.Pos.SelfClosing = off == end
		p.body.Paragraphs = append(p.body.Paragraphs, p.para)
		p.para = nil
	}
}

func readSectPr(d *xml.Decoder) (Section, error) {
	var s Section
	for {
		tok, err := d.Token()
		if err != nil {
			return s, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return s, nil
		case xml.StartElement:
			if isW(t.Name) {
				switch t.Name.Local {
				case "pgSz":
					w, err1 := twipsAttr(t, "w")
					h, err2 := twipsAttr(t, "h")
					if err := errors.Join(err1, err2); err != nil {
						return s, err
					}
					s.PageWidth, s.PageHeight, s.HasPageSize = w.Val, h.Val, w.Set && h.Set
					if o, _ := attr(t, "orient"); o == "landscape" || s.PageWidth > s.PageHeight {
						s.Landscape = true
					}
				case "pgMar":
					var errs []error
					get := func(k string) format.Twips {
						o, err := twipsAttr(t, k)
						errs = append(errs, err)
						return o.Val
					}
					s.MarginTop, s.MarginBottom = get("top"), get("bottom")
					s.MarginLeft, s.MarginRight = get("left"), get("right")
					if err := errors.Join(errs...); err != nil {
						return s, err
					}
					s.HasMargins = true
				}
			}
			if err := d.Skip(); err != nil {
				return s, err
			}
		}
	}
}
