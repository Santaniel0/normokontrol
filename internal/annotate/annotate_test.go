package annotate

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/docxtest"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/zipx"
)

func open(t *testing.T, data []byte) *ooxml.Document {
	t.Helper()
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

type comment struct {
	ID     string `xml:"id,attr"`
	Author string `xml:"author,attr"`
	Paras  []struct {
		Text string `xml:"r>t"`
	} `xml:"p"`
}

func readComments(t *testing.T, doc *ooxml.Document) []comment {
	t.Helper()
	name, ok := doc.RelatedPath(ooxml.RelComments)
	if !ok {
		t.Fatal("no comments relationship")
	}
	data, err := doc.Pkg.ReadPart(name)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Comments []comment `xml:"comment"`
	}
	if err := xml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c.Comments
}

// markers counts comment anchors per id in document.xml.
func markers(t *testing.T, data []byte) map[string]map[string]int {
	t.Helper()
	out := map[string]map[string]int{}
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("annotated document.xml is not well-formed: %v", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.HasPrefix(se.Name.Local, "comment") {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "id" {
				if out[a.Value] == nil {
					out[a.Value] = map[string]int{}
				}
				out[a.Value][se.Name.Local]++
			}
		}
	}
}

func rawEntries(t *testing.T, data []byte) (names []string, raw map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	raw = map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.OpenRaw()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, f.Name)
		raw[f.Name] = b
	}
	return names, raw
}

func TestRoundTripKeepsExistingComments(t *testing.T) {
	P, T := docxtest.P, docxtest.T
	body := P(`<w:jc w:val="both"/>`, `<w:commentRangeStart w:id="0"/>`, T("Абзац с замечанием руководителя"),
		`<w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/></w:r>`) + // 0
		P("", T("Второй абзац")) + // 1
		`<w:p w:rsidR="00AB12CD"/>` + // 2
		docxtest.Table([]string{P("", T("ячейка")), P("", T("вторая"))}) + // 3, 4
		P("", docxtest.Picture(1)) // 5
	src, err := docxtest.Build(docxtest.Doc{Body: body, Image: true,
		Comments: `<w:comment w:id="0" w:author="Научрук" w:initials="Н"><w:p><w:r><w:t>Перепишите вывод</w:t></w:r></w:p></w:comment>`})
	if err != nil {
		t.Fatal(err)
	}
	doc := open(t, src)
	notes := []Note{
		{Para: 0, Lines: []string{"Ошибка: шрифт & размер <14>"}},
		{Para: 2, Lines: []string{"пустой абзац"}},
		{Para: 3, Lines: []string{"ячейка"}},
		{Para: 0, Lines: []string{"второе замечание к тому же абзацу"}},
	}
	var out bytes.Buffer
	if err := Write(&out, doc, notes, Options{Author: "normokontrol", Initials: "НК"}); err != nil {
		t.Fatal(err)
	}

	if err := zipx.Verify(out.Bytes()); err != nil {
		t.Fatal(err)
	}
	got := open(t, out.Bytes())
	if len(got.Body.Paragraphs) != len(doc.Body.Paragraphs) {
		t.Fatalf("paragraphs: %d, want %d", len(got.Body.Paragraphs), len(doc.Body.Paragraphs))
	}
	for i, p := range got.Body.Paragraphs {
		if p.Text() != doc.Body.Paragraphs[i].Text() {
			t.Errorf("paragraph %d text changed: %q", i, p.Text())
		}
	}

	cs := readComments(t, got)
	if len(cs) != 4 {
		t.Fatalf("got %d comments, want 4", len(cs))
	}
	if cs[0].ID != "0" || cs[0].Author != "Научрук" || cs[0].Paras[0].Text != "Перепишите вывод" {
		t.Errorf("existing comment changed: %+v", cs[0])
	}
	if cs[1].ID != "1" || len(cs[1].Paras) != 2 || cs[1].Paras[0].Text != "Ошибка: шрифт & размер <14>" {
		t.Errorf("merged comment = %+v", cs[1])
	}
	m := markers(t, got.XML)
	for _, id := range []string{"0", "1", "2", "3"} {
		for _, kind := range []string{"commentRangeStart", "commentRangeEnd", "commentReference"} {
			if m[id][kind] != 1 {
				t.Errorf("id %s: %s appears %d times", id, kind, m[id][kind])
			}
		}
	}

	srcNames, srcRaw := rawEntries(t, src)
	outNames, outRaw := rawEntries(t, out.Bytes())
	if strings.Join(srcNames, ",") != strings.Join(outNames, ",") {
		t.Errorf("entry order changed:\n%v\n%v", srcNames, outNames)
	}
	for _, name := range srcNames {
		if name == "word/document.xml" || name == "word/comments.xml" {
			continue
		}
		if !bytes.Equal(srcRaw[name], outRaw[name]) {
			t.Errorf("%s is not byte-identical", name)
		}
	}
}

// withDescriptors repacks a package the way archive/zip's streaming writer
// and many other tools do: every entry followed by a data descriptor.
func withDescriptors(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(w, rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAddsCommentsPart(t *testing.T) {
	built, err := docxtest.Build(docxtest.Doc{Body: docxtest.P("", docxtest.T("Текст")), Image: true})
	if err != nil {
		t.Fatal(err)
	}
	src := withDescriptors(t, built)
	if zipx.Verify(src) == nil {
		t.Fatal("test input should use data descriptors")
	}
	var out bytes.Buffer
	if err := Write(&out, open(t, src), []Note{{Para: 0, Lines: []string{"замечание"}}}, Options{Author: "a"}); err != nil {
		t.Fatal(err)
	}
	// Copied entries must lose the descriptor of the source.
	if err := zipx.Verify(out.Bytes()); err != nil {
		t.Fatal(err)
	}
	got := open(t, out.Bytes())
	cs := readComments(t, got)
	if len(cs) != 1 || cs[0].ID != "0" {
		t.Fatalf("comments = %+v", cs)
	}
	ct, err := got.Pkg.ReadPart("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ct), `PartName="/word/comments.xml"`) {
		t.Error("content type override missing")
	}
	_, srcRaw := rawEntries(t, src)
	_, outRaw := rawEntries(t, out.Bytes())
	for _, name := range []string{"word/styles.xml", "word/theme/theme1.xml", "_rels/.rels"} {
		if !bytes.Equal(srcRaw[name], outRaw[name]) {
			t.Errorf("%s is not byte-identical", name)
		}
	}
}

func TestRootPrefix(t *testing.T) {
	tests := []struct {
		xml, want string
		ok        bool
	}{
		{`<?xml version="1.0"?><w:document xmlns:w="` + ooxml.NSWordMain + `"/>`, "w:", true},
		{`<ns0:document xmlns:ns0="` + ooxml.NSWordMain + `"/>`, "ns0:", true},
		{`<document xmlns="` + ooxml.NSWordMain + `"/>`, "", true},
		{`<w:document xmlns:w="http://purl.oclc.org/ooxml/wordprocessingml/main"/>`, "", false},
	}
	for _, tt := range tests {
		got, err := rootPrefix([]byte(tt.xml))
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("rootPrefix(%s) = %q, %v", tt.xml, got, err)
		}
	}
}

func TestSelfClosingRootGetsOpened(t *testing.T) {
	got, err := insertBeforeClose([]byte(`<?xml version="1.0"?><w:comments xmlns:w="x"/>`), "w:comments", "<w:comment/>")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `<?xml version="1.0"?><w:comments xmlns:w="x"><w:comment/></w:comments>` {
		t.Errorf("got %s", got)
	}
}

func TestNoteOutOfRange(t *testing.T) {
	src, err := docxtest.Build(docxtest.Doc{Body: docxtest.P("", docxtest.T("x"))})
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(io.Discard, open(t, src), []Note{{Para: 5}}, Options{}); err == nil {
		t.Error("expected an error")
	}
}
