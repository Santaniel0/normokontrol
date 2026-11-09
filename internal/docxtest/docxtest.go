// Package docxtest builds small but structurally valid .docx packages for
// tests and for the bundled samples. Element order inside pPr, rPr, sectPr
// and style follows the ECMA-376 schema because Word rejects files that
// violate it, even when every element is individually correct.
package docxtest

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"
)

// Doc describes a package. Empty fields get Word-like defaults.
type Doc struct {
	DocDefaults string // inner XML of w:docDefaults; "" means Word 2016 defaults (Calibri 11 via theme)
	Styles      string // w:style elements; "" means DefaultStyles
	Body        string // inner XML of w:body; a final w:sectPr is appended if missing
	Comments    string // w:comment elements; when set, comments.xml is added
	NoTheme     bool
	Image       bool // adds word/media/image1.png with relationship id "rIdImg1"
}

const (
	nsW   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsWP  = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	nsA   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsPic = "http://schemas.openxmlformats.org/drawingml/2006/picture"
	relNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)

// WordDocDefaults mirrors what Word 2016+ writes for a new blank document.
const WordDocDefaults = `<w:rPrDefault><w:rPr><w:rFonts w:asciiTheme="minorHAnsi" w:eastAsiaTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi" w:cstheme="minorBidi"/><w:sz w:val="22"/><w:szCs w:val="22"/><w:lang w:val="ru-RU" w:eastAsia="en-US" w:bidi="ar-SA"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="259" w:lineRule="auto"/></w:pPr></w:pPrDefault>`

// DefaultStyles are the built-in styles with the ids Russian Word uses.
const DefaultStyles = `<w:style w:type="paragraph" w:default="1" w:styleId="a"><w:name w:val="Normal"/><w:qFormat/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="1"><w:name w:val="heading 1"/><w:basedOn w:val="a"/><w:next w:val="a"/><w:qFormat/><w:pPr><w:keepNext/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="2"><w:name w:val="heading 2"/><w:basedOn w:val="a"/><w:next w:val="a"/><w:qFormat/><w:pPr><w:keepNext/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/></w:rPr></w:style>` +
	`<w:style w:type="character" w:default="1" w:styleId="a0"><w:name w:val="Default Paragraph Font"/><w:uiPriority w:val="1"/><w:semiHidden/><w:unhideWhenUsed/></w:style>`

// Build returns the zipped package.
func Build(d Doc) ([]byte, error) {
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes(d)},
		{"_rels/.rels", rels(rel{"rId1", "officeDocument", "word/document.xml"})},
		{"word/document.xml", document(d)},
		{"word/_rels/document.xml.rels", rels(docRels(d)...)},
		{"word/styles.xml", styles(d)},
	}
	if !d.NoTheme {
		parts = append(parts, struct{ name, body string }{"word/theme/theme1.xml", Theme})
	}
	if d.Comments != "" {
		parts = append(parts, struct{ name, body string }{"word/comments.xml",
			xmlDecl + `<w:comments xmlns:w="` + nsW + `">` + d.Comments + `</w:comments>`})
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mod := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, p := range parts {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: mod})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if d.Image {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: "word/media/image1.png", Method: zip.Store, Modified: mod})
		if err != nil {
			return nil, err
		}
		if err := png.Encode(w, picture()); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

type rel struct{ id, kind, target string }

func docRels(d Doc) []rel {
	rs := []rel{{"rId1", "styles", "styles.xml"}}
	if !d.NoTheme {
		rs = append(rs, rel{"rId2", "theme", "theme/theme1.xml"})
	}
	if d.Comments != "" {
		rs = append(rs, rel{"rId3", "comments", "comments.xml"})
	}
	if d.Image {
		rs = append(rs, rel{"rIdImg1", "image", "media/image1.png"})
	}
	return rs
}

func rels(rs ...rel) string {
	var b strings.Builder
	b.WriteString(xmlDecl + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rs {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s%s" Target="%s"/>`, r.id, relNS, r.kind, r.target)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func contentTypes(d Doc) string {
	const wml = "application/vnd.openxmlformats-officedocument.wordprocessingml."
	var b strings.Builder
	b.WriteString(xmlDecl + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	if d.Image {
		b.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	}
	b.WriteString(`<Override PartName="/word/document.xml" ContentType="` + wml + `document.main+xml"/>`)
	b.WriteString(`<Override PartName="/word/styles.xml" ContentType="` + wml + `styles+xml"/>`)
	if !d.NoTheme {
		b.WriteString(`<Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)
	}
	if d.Comments != "" {
		b.WriteString(`<Override PartName="/word/comments.xml" ContentType="` + wml + `comments+xml"/>`)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func document(d Doc) string {
	body := d.Body
	if !strings.HasSuffix(strings.TrimSpace(body), "</w:sectPr>") {
		body += Sect(SectA4())
	}
	return xmlDecl + `<w:document xmlns:w="` + nsW + `" xmlns:r="` + nsR + `" xmlns:wp="` + nsWP +
		`" xmlns:a="` + nsA + `" xmlns:pic="` + nsPic + `"><w:body>` + body + `</w:body></w:document>`
}

func styles(d Doc) string {
	defaults := d.DocDefaults
	if defaults == "" {
		defaults = WordDocDefaults
	}
	st := d.Styles
	if st == "" {
		st = DefaultStyles
	}
	return xmlDecl + `<w:styles xmlns:w="` + nsW + `"><w:docDefaults>` + defaults + `</w:docDefaults>` + st + `</w:styles>`
}

func picture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for x := 0; x < 8; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.Gray{Y: uint8(40 * (x % 4))})
		}
	}
	return img
}

// Escape escapes text for use inside w:t.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// P is a paragraph. pPr is the inner XML of w:pPr and may be empty.
func P(pPr string, runs ...string) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	if pPr != "" {
		b.WriteString("<w:pPr>" + pPr + "</w:pPr>")
	}
	for _, r := range runs {
		b.WriteString(r)
	}
	b.WriteString("</w:p>")
	return b.String()
}

// R is a run; rPr is the inner XML of w:rPr and may be empty.
func R(rPr, text string) string {
	s := "<w:r>"
	if rPr != "" {
		s += "<w:rPr>" + rPr + "</w:rPr>"
	}
	return s + `<w:t xml:space="preserve">` + Escape(text) + "</w:t></w:r>"
}

// T is a run without properties.
func T(text string) string { return R("", text) }

// PageBreak is a run with a page break.
const PageBreak = `<w:r><w:br w:type="page"/></w:r>`

// Picture is an inline image run. id must be unique within the document.
func Picture(id int) string {
	return fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="2743200" cy="1371600"/><wp:effectExtent l="0" t="0" r="0" b="0"/>`+
		`<wp:docPr id="%d" name="Picture %d"/><wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic>`+
		`<pic:nvPicPr><pic:cNvPr id="%d" name="image1.png"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="rIdImg1"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="2743200" cy="1371600"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`, id, id, id)
}

// Table is a bordered table; each row is a list of already built cell paragraphs.
func Table(rows ...[]string) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		fmt.Fprintf(&b, `<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, side)
	}
	b.WriteString(`</w:tblBorders><w:tblLook w:val="04A0"/></w:tblPr><w:tblGrid>`)
	cols := 0
	if len(rows) > 0 {
		cols = len(rows[0])
	}
	for i := 0; i < cols; i++ {
		fmt.Fprintf(&b, `<w:gridCol w:w="%d"/>`, 9355/cols)
	}
	b.WriteString(`</w:tblGrid>`)
	for _, row := range rows {
		b.WriteString("<w:tr>")
		for _, cell := range row {
			fmt.Fprintf(&b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`, 9355/cols, cell)
		}
		b.WriteString("</w:tr>")
	}
	b.WriteString("</w:tbl>")
	return b.String()
}

// Page describes a section's page geometry in millimetres.
type Page struct {
	WidthMM, HeightMM                float64
	TopMM, RightMM, BottomMM, LeftMM float64
	Landscape, TitlePage             bool
}

// SectA4 is an A4 page with GOST 7.32-2017 margins.
func SectA4() Page {
	return Page{WidthMM: 210, HeightMM: 297, TopMM: 20, RightMM: 15, BottomMM: 20, LeftMM: 30}
}

func tw(mm float64) int { return int(mm*1440/25.4 + 0.5) }

// Sect is a w:sectPr element.
func Sect(p Page) string {
	orient := ""
	if p.Landscape {
		orient = ` w:orient="landscape"`
	}
	title := ""
	if p.TitlePage {
		title = "<w:titlePg/>"
	}
	return fmt.Sprintf(`<w:sectPr><w:pgSz w:w="%d" w:h="%d"%s/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="708" w:footer="708" w:gutter="0"/><w:cols w:space="708"/>%s<w:docGrid w:linePitch="360"/></w:sectPr>`,
		tw(p.WidthMM), tw(p.HeightMM), orient, tw(p.TopMM), tw(p.RightMM), tw(p.BottomMM), tw(p.LeftMM), title)
}

// Theme is a minimal schema-complete theme with Calibri / Calibri Light.
const Theme = xmlDecl + `<a:theme xmlns:a="` + nsA + `" name="Office Theme"><a:themeElements>` +
	`<a:clrScheme name="Office"><a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="44546A"/></a:dk2><a:lt2><a:srgbClr val="E7E6E6"/></a:lt2><a:accent1><a:srgbClr val="4472C4"/></a:accent1>` +
	`<a:accent2><a:srgbClr val="ED7D31"/></a:accent2><a:accent3><a:srgbClr val="A5A5A5"/></a:accent3><a:accent4><a:srgbClr val="FFC000"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="5B9BD5"/></a:accent5><a:accent6><a:srgbClr val="70AD47"/></a:accent6><a:hlink><a:srgbClr val="0563C1"/></a:hlink>` +
	`<a:folHlink><a:srgbClr val="954F72"/></a:folHlink></a:clrScheme>` +
	`<a:fontScheme name="Office"><a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
	`<a:fmtScheme name="Office"><a:fillStyleLst>` + fill3 + `</a:fillStyleLst><a:lnStyleLst>` +
	`<a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>` +
	`</a:lnStyleLst><a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`<a:bgFillStyleLst>` + fill3 + `</a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`

const fill3 = `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`
