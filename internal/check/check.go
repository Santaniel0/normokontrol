// Package check ties the pipeline together: open, classify, run rules,
// and optionally write an annotated copy.
package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Santaniel0/normokontrol/internal/annotate"
	"github.com/Santaniel0/normokontrol/internal/model"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/profile"
	"github.com/Santaniel0/normokontrol/internal/rules"
)

// MaxFileBytes caps the size of the .docx file itself.
const MaxFileBytes = 200 << 20

// ErrFileTooLarge is returned for files above MaxFileBytes.
var ErrFileTooLarge = errors.New("file too large")

// Item is a finding with a human-oriented location.
type Item struct {
	rules.Finding
	Loc model.Location
}

// Stats describe what was checked.
type Stats struct {
	Sections, Paragraphs, Headings, Figures, Tables, Sources int
	Errors, Warnings                                         int
}

// Result is the outcome for one document.
type Result struct {
	File     string
	Profile  string
	Stats    Stats
	Findings []Item
}

// Checked keeps the parsed package so that an annotated copy can be written.
type Checked struct {
	Result Result
	doc    *ooxml.Document
}

// File checks a document on disk. The file is read into memory so that the
// annotated copy can be written without keeping it open.
func File(ctx context.Context, path string, p *profile.Profile) (*Checked, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("%s: %w", path, ErrFileTooLarge)
	}
	return Bytes(ctx, filepath.Base(path), data, p)
}

// Bytes checks a document held in memory.
func Bytes(ctx context.Context, name string, data []byte, p *profile.Profile) (*Checked, error) {
	pkg, err := ooxml.Open(bytes.NewReader(data), int64(len(data)), ooxml.DefaultLimits())
	if err != nil {
		return nil, err
	}
	doc, err := ooxml.Load(pkg)
	if err != nil {
		return nil, err
	}
	m := model.Build(doc, model.Options{
		Structural:   append(append([]string{}, p.Structure.Order...), p.Structure.Required...),
		Bibliography: p.Structure.Bibliography,
	})
	findings, err := rules.Run(ctx, m, p)
	if err != nil {
		return nil, err
	}
	res := Result{File: name, Profile: p.Name, Stats: stats(m)}
	for _, f := range findings {
		res.Findings = append(res.Findings, Item{Finding: f, Loc: m.Locate(f.Para)})
		if f.Severity == rules.SeverityError {
			res.Stats.Errors++
		} else {
			res.Stats.Warnings++
		}
	}
	return &Checked{Result: res, doc: doc}, nil
}

func stats(m *model.Document) Stats {
	s := Stats{Sections: len(m.Sections), Tables: m.Tables, Sources: len(m.BibEntries())}
	for _, p := range m.Paras {
		switch p.Role {
		case model.RoleEmpty:
			continue
		case model.RoleHeading:
			s.Headings++
		case model.RoleFigure:
			s.Figures++
		}
		s.Paragraphs++
	}
	return s
}

// maxCommentsPerMessage keeps a document with one style mistake from
// getting hundreds of identical comments.
const maxCommentsPerMessage = 20

// Annotate writes a copy of the document with one Word comment per
// affected paragraph and returns the number of comments.
func (c *Checked) Annotate(w io.Writer, lines func(Item) []string) (int, error) {
	perMessage := map[string]int{}
	byPara := map[int]*annotate.Note{}
	var notes []*annotate.Note
	for _, it := range c.Result.Findings {
		key := string(it.Rule) + "\x00" + it.Message
		if perMessage[key] >= maxCommentsPerMessage {
			continue
		}
		perMessage[key]++
		para := max(it.Para, 0)
		if len(c.doc.Body.Paragraphs) == 0 {
			break
		}
		n, ok := byPara[para]
		if !ok {
			n = &annotate.Note{Para: para}
			byPara[para] = n
			notes = append(notes, n)
		}
		n.Lines = append(n.Lines, lines(it)...)
	}
	out := make([]annotate.Note, len(notes))
	for i, n := range notes {
		out[i] = *n
	}
	if err := annotate.Write(w, c.doc, out, annotate.Options{Author: msg.CommentAuthor, Initials: msg.CommentInitials}); err != nil {
		return 0, err
	}
	return len(out), nil
}
