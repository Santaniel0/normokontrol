package ooxml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// Relationship types are matched by their last path segment so that both
// transitional and strict OOXML URIs work.
const (
	RelOfficeDocument = "officeDocument"
	RelStyles         = "styles"
	RelTheme          = "theme"
	RelComments       = "comments"
)

// Relationship is one entry of a .rels part.
type Relationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// Kind returns the last segment of the relationship type URI.
func (r Relationship) Kind() string {
	return r.Type[strings.LastIndex(r.Type, "/")+1:]
}

type relsXML struct {
	Rels []Relationship `xml:"Relationship"`
}

// ParseRels parses a relationships part.
func ParseRels(data []byte) ([]Relationship, error) {
	var r relsXML
	if err := xml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedXML, err)
	}
	return r.Rels, nil
}

// Document is a loaded .docx: the raw main part plus everything parsed from it.
type Document struct {
	Pkg    *Package
	Path   string // main document part, usually word/document.xml
	XML    []byte
	Rels   []Relationship
	Body   *Body
	Styles format.StyleSheet
	Theme  format.Theme
}

// Load reads and parses the main document, its styles and theme.
func Load(p *Package) (*Document, error) {
	doc := &Document{Pkg: p, Path: "word/document.xml"}
	if data, err := p.ReadPart("_rels/.rels"); err == nil {
		rels, err := ParseRels(data)
		if err != nil {
			return nil, fmt.Errorf("_rels/.rels: %w", err)
		}
		for _, r := range rels {
			if r.Kind() == RelOfficeDocument {
				doc.Path = ResolveTarget("", r.Target)
			}
		}
	}
	var err error
	if doc.XML, err = p.ReadPart(doc.Path); err != nil {
		if errors.Is(err, ErrPartMissing) {
			return nil, fmt.Errorf("%w: %w", ErrNotDOCX, err)
		}
		return nil, err
	}
	if data, err := p.ReadPart(RelsPath(doc.Path)); err == nil {
		if doc.Rels, err = ParseRels(data); err != nil {
			return nil, fmt.Errorf("%s: %w", RelsPath(doc.Path), err)
		}
	} else if !errors.Is(err, ErrPartMissing) {
		return nil, err
	}
	if doc.Body, err = ParseDocument(doc.XML); err != nil {
		return nil, err
	}
	doc.Styles.Styles = map[format.StyleID]format.Style{}
	if data, ok, err := doc.related(RelStyles); err != nil {
		return nil, err
	} else if ok {
		if doc.Styles, err = ParseStyles(data); err != nil {
			return nil, err
		}
	}
	if data, ok, err := doc.related(RelTheme); err != nil {
		return nil, err
	} else if ok {
		if doc.Theme, err = ParseTheme(data); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

// RelatedPath returns the part name of the first relationship of a kind.
func (d *Document) RelatedPath(kind string) (string, bool) {
	for _, r := range d.Rels {
		if r.Kind() == kind && r.TargetMode != "External" {
			return ResolveTarget(d.Path, r.Target), true
		}
	}
	return "", false
}

func (d *Document) related(kind string) ([]byte, bool, error) {
	name, ok := d.RelatedPath(kind)
	if !ok {
		return nil, false, nil
	}
	data, err := d.Pkg.ReadPart(name)
	if errors.Is(err, ErrPartMissing) {
		return nil, false, nil
	}
	return data, err == nil, err
}
