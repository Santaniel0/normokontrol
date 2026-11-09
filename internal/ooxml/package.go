// Package ooxml reads the parts of a .docx file that matter for formatting
// checks: the package itself, relationships, document.xml, styles and theme.
package ooxml

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var (
	// ErrNotDOCX means the input is not a zip package with a Word document.
	ErrNotDOCX = errors.New("not a .docx file")
	// ErrLegacyDOC means the input is an OLE compound file: a Word 97-2003
	// .doc or a password-protected .docx, which Word also stores that way.
	ErrLegacyDOC = errors.New("legacy .doc or encrypted document")
	// ErrTooLarge means the package exceeds the configured limits.
	ErrTooLarge = errors.New("document exceeds size limits")
	// ErrPartMissing means a required part is absent from the package.
	ErrPartMissing = errors.New("package part missing")
)

// Limits bound how much a single document may decompress to. They protect
// batch runs from zip bombs and from accidentally huge files.
type Limits struct {
	MaxEntries    int
	MaxPartBytes  int64
	MaxTotalBytes int64
}

// DefaultLimits are generous for a thesis with photos: images are counted in
// the total but never decompressed.
func DefaultLimits() Limits {
	return Limits{MaxEntries: 5000, MaxPartBytes: 64 << 20, MaxTotalBytes: 512 << 20}
}

// Package is an opened .docx. It is not safe for concurrent use.
type Package struct {
	files  []*zip.File
	byName map[string]*zip.File
	limits Limits
}

var oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// Open validates the zip directory against the limits.
func Open(r io.ReaderAt, size int64, lim Limits) (*Package, error) {
	head := make([]byte, len(oleMagic))
	if n, _ := r.ReadAt(head, 0); n == len(head) && bytes.Equal(head, oleMagic) {
		return nil, ErrLegacyDOC
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotDOCX, err)
	}
	if len(zr.File) > lim.MaxEntries {
		return nil, fmt.Errorf("%w: %d entries", ErrTooLarge, len(zr.File))
	}
	p := &Package{files: zr.File, byName: make(map[string]*zip.File, len(zr.File)), limits: lim}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > uint64(lim.MaxTotalBytes) {
			return nil, fmt.Errorf("%w: more than %d bytes uncompressed", ErrTooLarge, lim.MaxTotalBytes)
		}
		p.byName[strings.TrimPrefix(f.Name, "/")] = f
	}
	if _, ok := p.byName["[Content_Types].xml"]; !ok {
		return nil, fmt.Errorf("%w: no [Content_Types].xml", ErrNotDOCX)
	}
	return p, nil
}

// Files returns the zip entries in their original order.
func (p *Package) Files() []*zip.File { return p.files }

// Has reports whether a part exists.
func (p *Package) Has(name string) bool {
	_, ok := p.byName[name]
	return ok
}

// ReadPart returns the decompressed bytes of a part. The size header is not
// trusted: reading stops one byte past the limit.
func (p *Package) ReadPart(name string) ([]byte, error) {
	f, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPartMissing, name)
	}
	if f.UncompressedSize64 > uint64(p.limits.MaxPartBytes) {
		return nil, fmt.Errorf("%w: part %s", ErrTooLarge, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, p.limits.MaxPartBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > p.limits.MaxPartBytes {
		return nil, fmt.Errorf("%w: part %s", ErrTooLarge, name)
	}
	return data, nil
}

// RelsPath returns the relationships part that belongs to a part.
func RelsPath(part string) string {
	dir, file := path.Split(part)
	return dir + "_rels/" + file + ".rels"
}

// ResolveTarget turns a relationship target into a package part name.
func ResolveTarget(source, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Clean(path.Join(path.Dir(source), target))
}
