package ooxml_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/docxtest"
	"github.com/Santaniel0/normokontrol/internal/format"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
)

func load(t *testing.T, d docxtest.Doc) *ooxml.Document {
	t.Helper()
	data, err := docxtest.Build(d)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ooxml.Load(pkg)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestOpenRejects(t *testing.T) {
	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 512)...)
	tests := []struct {
		name string
		data []byte
		lim  ooxml.Limits
		want error
	}{
		{"legacy doc", ole, ooxml.DefaultLimits(), ooxml.ErrLegacyDOC},
		{"plain text", []byte("Курсовая работа"), ooxml.DefaultLimits(), ooxml.ErrNotDOCX},
		{"zip without content types", zipOf(t, map[string]string{"a.txt": "x"}), ooxml.DefaultLimits(), ooxml.ErrNotDOCX},
		{"too many entries", zipOf(t, map[string]string{"[Content_Types].xml": "", "a": "", "b": ""}),
			ooxml.Limits{MaxEntries: 2, MaxPartBytes: 1 << 20, MaxTotalBytes: 1 << 20}, ooxml.ErrTooLarge},
		{"too much data", zipOf(t, map[string]string{"[Content_Types].xml": strings.Repeat("x", 2000)}),
			ooxml.Limits{MaxEntries: 10, MaxPartBytes: 1 << 20, MaxTotalBytes: 1000}, ooxml.ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ooxml.Open(bytes.NewReader(tt.data), int64(len(tt.data)), tt.lim)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReadPartLimit(t *testing.T) {
	data := zipOf(t, map[string]string{"[Content_Types].xml": "<Types/>", "big.xml": strings.Repeat("a", 5000)})
	pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.Limits{MaxEntries: 10, MaxPartBytes: 1000, MaxTotalBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.ReadPart("big.xml"); !errors.Is(err, ooxml.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if _, err := pkg.ReadPart("nope.xml"); !errors.Is(err, ooxml.ErrPartMissing) {
		t.Fatalf("err = %v, want ErrPartMissing", err)
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseDocumentContent(t *testing.T) {
	P, R, T := docxtest.P, docxtest.R, docxtest.T
	twoCm := docxtest.SectA4()
	twoCm.RightMM = 10
	body := P(`<w:pStyle w:val="1"/>`, T("ВВЕДЕНИЕ")) +
		P(`<w:spacing w:line="360" w:lineRule="auto"/><w:ind w:firstLine="709"/><w:jc w:val="both"/>`,
			T("Текст "), R(`<w:b/>`, "жирный"), `<w:hyperlink r:id="x"><w:r><w:t>ссылка</w:t></w:r></w:hyperlink>`) +
		P(`<w:ind w:left="720" w:hanging="360"/><w:spacing w:line="320" w:lineRule="exact"/>`,
			`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r>`,
			`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, T("7"), `<w:r><w:fldChar w:fldCharType="end"/></w:r>`,
			`<w:del w:id="1" w:author="x"><w:r><w:delText>удалено</w:delText></w:r></w:del>`) +
		P(`<w:sectPr>`+strings.TrimPrefix(docxtest.Sect(twoCm), `<w:sectPr>`), T("конец раздела")) +
		docxtest.Table([]string{P("", T("ячейка")), P("", T("2"))}) +
		P("", docxtest.Picture(1)) +
		P("", `<mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice Requires="wps"><w:r><w:t>надпись</w:t></w:r></mc:Choice><mc:Fallback><w:r><w:t>надпись</w:t></w:r></mc:Fallback></mc:AlternateContent>`) +
		`<w:p/>` +
		P("", docxtest.PageBreak) +
		P("", T("Формула: "), `<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:r><m:t>x</m:t></m:r></m:oMath>`)
	doc := load(t, docxtest.Doc{Body: body, Image: true})
	ps := doc.Body.Paragraphs
	if len(ps) != 11 {
		t.Fatalf("got %d paragraphs, want 11", len(ps))
	}
	texts := []string{"ВВЕДЕНИЕ", "Текст жирныйссылка", "7", "конец раздела", "ячейка", "2", "", "", "", "", "Формула: "}
	for i, want := range texts {
		if got := ps[i].Text(); got != want {
			t.Errorf("paragraph %d text = %q, want %q", i, got, want)
		}
	}
	if ps[0].StyleID != "1" {
		t.Errorf("style = %q", ps[0].StyleID)
	}
	p1 := ps[1].Props
	if p1.FirstLine.Val != 709 || p1.Align.Val != format.AlignJustify || p1.Line.Val != (format.LineSpacing{Value: 360, Rule: format.LineAuto}) {
		t.Errorf("paragraph 1 props = %+v", p1)
	}
	if !ps[1].Runs[1].Props.Bold.Val {
		t.Error("second run should be bold")
	}
	p2 := ps[2].Props
	if p2.FirstLine.Val != -360 || p2.IndLeft.Val != 720 || p2.Line.Val.Rule != format.LineExact {
		t.Errorf("paragraph 2 props = %+v", p2)
	}
	if len(doc.Body.Sections) != 2 || doc.Body.Sections[0].MarginRight != 567 || doc.Body.Sections[1].MarginRight != 850 {
		t.Errorf("sections = %+v", doc.Body.Sections)
	}
	if ps[3].Section != 0 || ps[4].Section != 1 {
		t.Errorf("section of paragraph 3 = %d, 4 = %d", ps[3].Section, ps[4].Section)
	}
	if ps[4].Table != 1 || ps[5].Table != 1 || ps[3].Table != 0 || ps[6].Table != 0 {
		t.Error("table membership is wrong")
	}
	if !ps[6].Drawing || !ps[7].Drawing {
		t.Error("picture and text box paragraphs must be marked as drawings")
	}
	if !ps[8].Pos.SelfClosing || ps[8].Pos.AnchorStart != ps[8].Pos.End {
		t.Errorf("self-closing span = %+v", ps[8].Pos)
	}
	if !ps[9].PageBreak || !ps[10].Math {
		t.Error("page break / math flags missing")
	}
	raw := string(doc.XML)
	for i, p := range ps[:8] {
		if !strings.HasPrefix(raw[p.Pos.Start:], "<w:p>") || raw[p.Pos.AnchorEnd:p.Pos.End] != "</w:p>" {
			t.Errorf("paragraph %d span does not cover <w:p>...</w:p>", i)
		}
	}
	if !strings.HasSuffix(raw[:ps[1].Pos.AnchorStart], "</w:pPr>") {
		t.Error("anchor must follow </w:pPr>")
	}
	if !strings.HasSuffix(raw[:ps[6].Pos.AnchorStart], "<w:p>") {
		t.Error("anchor must follow the start tag when there is no pPr")
	}
}

func TestStylesAndTheme(t *testing.T) {
	doc := load(t, docxtest.Doc{Body: docxtest.P("", docxtest.T("x"))})
	if doc.Theme.Minor != "Calibri" || doc.Theme.Major != "Calibri Light" {
		t.Errorf("theme = %+v", doc.Theme)
	}
	if got := doc.Styles.DefaultRun.FontHAnsi; !got.Set || got.Val.Theme != format.ThemeMinor {
		t.Errorf("docDefaults hAnsi = %+v", got)
	}
	if got := doc.Styles.DefaultRun.Size.Val; got != 22 {
		t.Errorf("docDefaults size = %d", got)
	}
	h := doc.Styles.Styles["1"]
	if h.Name != "heading 1" || h.BasedOn != "a" || h.Para.OutlineLevel.Val != 0 || !h.Run.Bold.Val {
		t.Errorf("heading 1 = %+v", h)
	}
	if !doc.Styles.Styles["a"].Default || doc.Styles.Styles["a0"].Type != format.StyleCharacter {
		t.Error("default styles not recognised")
	}
}

func TestMissingFinalSection(t *testing.T) {
	body, err := ooxml.ParseDocument([]byte(`<w:document xmlns:w="` + ooxml.NSWordMain + `"><w:body><w:p><w:r><w:t>x</w:t></w:r></w:p></w:body></w:document>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Sections) != 1 || body.Sections[0].HasMargins {
		t.Errorf("sections = %+v", body.Sections)
	}
}

func TestMalformedXML(t *testing.T) {
	_, err := ooxml.ParseDocument([]byte(`<w:document xmlns:w="` + ooxml.NSWordMain + `"><w:body><w:p>`))
	if err == nil {
		t.Fatal("expected an error for truncated XML")
	}
}

func FuzzOpenAndLoad(f *testing.F) {
	good, err := docxtest.Build(docxtest.Doc{Body: docxtest.P("", docxtest.T("текст"))})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.Limits{MaxEntries: 50, MaxPartBytes: 1 << 20, MaxTotalBytes: 4 << 20})
		if err != nil {
			return
		}
		_, _ = ooxml.Load(pkg)
	})
}
