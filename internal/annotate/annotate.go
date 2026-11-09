// Package annotate writes a copy of a .docx with Word comments attached to
// paragraphs.
//
// The comment anchors are spliced into the original document.xml bytes at
// offsets recorded by the parser. Re-encoding the XML with encoding/xml is
// not an option: it does not preserve namespace prefixes, and Word refuses
// documents whose w: prefix has been rewritten. Every part that does not
// need to change is copied as raw compressed bytes.
package annotate

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/zipx"
)

// ErrUnsupported means the document uses XML the splicer cannot handle
// safely, for example Strict OOXML namespaces.
var ErrUnsupported = errors.New("document structure not supported for annotation")

// Note is the text of one comment attached to a paragraph.
type Note struct {
	Para  int
	Lines []string
}

// Options set the comment author shown in Word.
type Options struct {
	Author   string
	Initials string
}

const (
	relComments = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	ctComments  = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
)

// Write streams the annotated package to w.
func Write(w io.Writer, doc *ooxml.Document, notes []Note, opt Options) error {
	notes = merge(notes)
	for _, n := range notes {
		if n.Para < 0 || n.Para >= len(doc.Body.Paragraphs) {
			return fmt.Errorf("note for paragraph %d: out of range", n.Para)
		}
	}
	prefix, err := rootPrefix(doc.XML)
	if err != nil {
		return err
	}
	replaced := map[string][]byte{}
	var added []string

	commentsPath, hasComments := doc.RelatedPath(ooxml.RelComments)
	hasComments = hasComments && doc.Pkg.Has(commentsPath)
	var comments []byte
	nextID := 0
	if hasComments {
		if comments, err = doc.Pkg.ReadPart(commentsPath); err != nil {
			return err
		}
		if nextID, err = maxCommentID(comments); err != nil {
			return err
		}
		nextID++
	} else {
		commentsPath = freeName(doc.Pkg, path.Join(path.Dir(doc.Path), "comments.xml"))
		comments = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<w:comments xmlns:w="` + ooxml.NSWordMain + `"></w:comments>`)
		added = append(added, commentsPath)
	}

	ids := make([]int, len(notes))
	for i := range notes {
		ids[i] = nextID + i
	}
	replaced[doc.Path] = splice(doc, notes, ids, prefix)
	if replaced[commentsPath], err = appendComments(comments, notes, ids, opt); err != nil {
		return err
	}
	if !hasComments {
		if err := addRelationship(doc, commentsPath, replaced, &added); err != nil {
			return err
		}
		ct, err := doc.Pkg.ReadPart("[Content_Types].xml")
		if err != nil {
			return err
		}
		override := `<Override PartName="/` + commentsPath + `" ContentType="` + ctComments + `"/>`
		if replaced["[Content_Types].xml"], err = insertBeforeClose(ct, "Types", override); err != nil {
			return err
		}
	}
	return writeZip(w, doc.Pkg, replaced, added)
}

// merge folds notes for the same paragraph into one comment.
func merge(notes []Note) []Note {
	byPara := map[int]int{}
	var out []Note
	for _, n := range notes {
		if i, ok := byPara[n.Para]; ok {
			out[i].Lines = append(out[i].Lines, n.Lines...)
			continue
		}
		byPara[n.Para] = len(out)
		out = append(out, Note{Para: n.Para, Lines: append([]string(nil), n.Lines...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Para < out[j].Para })
	return out
}

// rootPrefix finds the prefix bound to the WordprocessingML namespace on the
// root element: "w:" almost always, "" for a default namespace.
func rootPrefix(data []byte) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.RawToken()
		if err != nil {
			return "", fmt.Errorf("%w: no root element: %w", ErrUnsupported, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range se.Attr {
			if a.Value != ooxml.NSWordMain {
				continue
			}
			if a.Name.Space == "xmlns" {
				return a.Name.Local + ":", nil
			}
			if a.Name.Space == "" && a.Name.Local == "xmlns" {
				return "", nil
			}
		}
		return "", fmt.Errorf("%w: WordprocessingML namespace is not declared on the root", ErrUnsupported)
	}
}

type edit struct {
	from, to int64 // bytes [from, to) are replaced by text
	text     string
}

func splice(doc *ooxml.Document, notes []Note, ids []int, w string) []byte {
	var edits []edit
	for i, n := range notes {
		pos := doc.Body.Paragraphs[n.Para].Pos
		id := strconv.Itoa(ids[i])
		start := `<` + w + `commentRangeStart ` + w + `id="` + id + `"/>`
		end := `<` + w + `commentRangeEnd ` + w + `id="` + id + `"/><` + w + `r><` + w + `commentReference ` + w + `id="` + id + `"/></` + w + `r>`
		if pos.SelfClosing {
			open := strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(string(doc.XML[pos.Start:pos.End]), "/>")), "/") + ">"
			edits = append(edits, edit{pos.Start, pos.End, open + start + end + "</" + w + "p>"})
			continue
		}
		edits = append(edits, edit{pos.AnchorStart, pos.AnchorStart, start}, edit{pos.AnchorEnd, pos.AnchorEnd, end})
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].from < edits[j].from })
	var b bytes.Buffer
	b.Grow(len(doc.XML) + 200*len(notes))
	var at int64
	for _, e := range edits {
		b.Write(doc.XML[at:e.from])
		b.WriteString(e.text)
		at = e.to
	}
	b.Write(doc.XML[at:])
	return b.Bytes()
}

func maxCommentID(data []byte) (int, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	maxID := -1
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return maxID, nil
		}
		if err != nil {
			return 0, fmt.Errorf("comments.xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "comment" {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "id" {
				if n, err := strconv.Atoi(a.Value); err == nil && n > maxID {
					maxID = n
				}
			}
		}
	}
}

func appendComments(data []byte, notes []Note, ids []int, opt Options) ([]byte, error) {
	w, err := rootPrefix(data)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for i, n := range notes {
		fmt.Fprintf(&b, `<%scomment %sid="%d" %sauthor="%s" %sinitials="%s">`, w, w, ids[i], w, escape(opt.Author), w, escape(opt.Initials))
		for _, line := range n.Lines {
			fmt.Fprintf(&b, `<%sp><%sr><%st xml:space="preserve">%s</%st></%sr></%sp>`, w, w, w, escape(line), w, w, w)
		}
		fmt.Fprintf(&b, `</%scomment>`, w)
	}
	return insertBeforeClose(data, w+"comments", b.String())
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// insertBeforeClose puts text right before the closing tag of the root
// element, turning a self-closing root into an open one if needed.
func insertBeforeClose(data []byte, root, text string) ([]byte, error) {
	closing := "</" + root + ">"
	if i := bytes.LastIndex(data, []byte(closing)); i >= 0 {
		return slicesConcat(data[:i], []byte(text), data[i:]), nil
	}
	selfClosing := regexp.MustCompile(`<` + regexp.QuoteMeta(root) + `\b[^>]*?/>`)
	if loc := selfClosing.FindIndex(data); loc != nil {
		open := bytes.TrimSuffix(data[loc[0]:loc[1]], []byte("/>"))
		return slicesConcat(data[:loc[0]], open, []byte(">"+text+closing), data[loc[1]:]), nil
	}
	return nil, fmt.Errorf("%w: no %s element", ErrUnsupported, root)
}

func slicesConcat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func freeName(pkg *ooxml.Package, name string) string {
	for i := 2; pkg.Has(name); i++ {
		name = strings.TrimSuffix(name, path.Ext(name))
		name = strings.TrimRight(name, "0123456789") + strconv.Itoa(i) + ".xml"
	}
	return name
}

func addRelationship(doc *ooxml.Document, commentsPath string, replaced map[string][]byte, added *[]string) error {
	relsPath := ooxml.RelsPath(doc.Path)
	used := map[string]bool{}
	for _, r := range doc.Rels {
		used[r.ID] = true
	}
	id := "rIdNk1"
	for i := 2; used[id]; i++ {
		id = "rIdNk" + strconv.Itoa(i)
	}
	target, err := relTarget(doc.Path, commentsPath)
	if err != nil {
		return err
	}
	rel := `<Relationship Id="` + id + `" Type="` + relComments + `" Target="` + target + `"/>`
	data, err := doc.Pkg.ReadPart(relsPath)
	if errors.Is(err, ooxml.ErrPartMissing) {
		replaced[relsPath] = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rel + `</Relationships>`)
		*added = append(*added, relsPath)
		return nil
	}
	if err != nil {
		return err
	}
	replaced[relsPath], err = insertBeforeClose(data, "Relationships", rel)
	return err
}

func relTarget(source, target string) (string, error) {
	dir := path.Dir(source)
	if dir == "." {
		return target, nil
	}
	if rest, ok := strings.CutPrefix(target, dir+"/"); ok {
		return rest, nil
	}
	return "/" + target, nil
}

// writeZip copies the package in its original entry order. Untouched
// entries are copied raw, so their compressed bytes stay identical; zipx
// keeps CRC and sizes in the local headers, which LibreOffice requires.
func writeZip(w io.Writer, pkg *ooxml.Package, replaced map[string][]byte, added []string) error {
	zw := zip.NewWriter(w)
	isAdded := map[string]bool{}
	for _, a := range added {
		isAdded[a] = true
	}
	for _, f := range pkg.Files() {
		name := strings.TrimPrefix(f.Name, "/")
		if data, ok := replaced[name]; ok && !isAdded[name] {
			if err := zipx.Write(zw, f.Name, zip.Deflate, f.Modified, data); err != nil {
				return err
			}
			continue
		}
		if err := zipx.Copy(zw, f); err != nil {
			return err
		}
	}
	// New parts take the timestamp of the first entry so that the output
	// does not depend on the clock.
	var mod time.Time
	if files := pkg.Files(); len(files) > 0 {
		mod = files[0].Modified
	}
	for _, name := range added {
		if err := zipx.Write(zw, name, zip.Deflate, mod, replaced[name]); err != nil {
			return err
		}
	}
	return zw.Close()
}
