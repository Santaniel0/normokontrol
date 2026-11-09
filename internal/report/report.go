// Package report renders check results as Russian text or versioned JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/check"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/rules"
)

// SchemaVersion changes whenever the JSON layout changes incompatibly.
const SchemaVersion = 1

type group struct {
	rule     rules.ID
	severity rules.Severity
	message  string
	items    []check.Item
}

// groups folds identical messages together so that forty paragraphs with
// the same indent read as one line with a count.
func groups(items []check.Item) []*group {
	index := map[string]*group{}
	var out []*group
	for _, it := range items {
		key := fmt.Sprintf("%s\x00%d\x00%s", it.Rule, it.Severity, it.Message)
		g, ok := index[key]
		if !ok {
			g = &group{rule: it.Rule, severity: it.Severity, message: it.Message}
			index[key] = g
			out = append(out, g)
		}
		g.items = append(g.items, it)
	}
	return out
}

// Text writes the human report.
func Text(w io.Writer, r check.Result) error {
	var b strings.Builder
	s := r.Stats
	b.WriteString(msg.ProfileLine(r.Profile) + "\n")
	b.WriteString(msg.DocumentLine(r.File, s.Sections, s.Paragraphs, s.Figures, s.Tables, s.Sources) + "\n")
	gs := groups(r.Findings)
	for _, sev := range []rules.Severity{rules.SeverityError, rules.SeverityWarning} {
		var sel []*group
		total := 0
		for _, g := range gs {
			if g.severity == sev {
				sel = append(sel, g)
				total += len(g.items)
			}
		}
		if len(sel) == 0 {
			continue
		}
		title := msg.ErrorsTitle
		if sev == rules.SeverityWarning {
			title = msg.WarningsTitle
		}
		fmt.Fprintf(&b, "\n%s (%d)\n", title, total)
		for _, g := range sel {
			writeGroup(&b, g)
		}
	}
	if len(r.Findings) == 0 {
		b.WriteString("\n" + msg.NoFindings + "\n")
	}
	b.WriteString("\n" + msg.Total(s.Errors, s.Warnings) + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func writeGroup(b *strings.Builder, g *group) {
	first := g.items[0]
	const indent = "                    "
	fmt.Fprintf(b, "  %-17s %s", g.rule, g.message)
	if len(g.items) > 1 {
		fmt.Fprintf(b, " — %s", msg.Times(len(g.items)))
		fmt.Fprintf(b, "\n%s%s: %s", indent, msg.FirstLabel, msg.Where(first.Loc))
	} else {
		fmt.Fprintf(b, "\n%s%s", indent, msg.Where(first.Loc))
	}
	if first.Detail != "" {
		fmt.Fprintf(b, "\n%s%s", indent, first.Detail)
	}
	b.WriteString("\n")
}

// CommentLines formats a finding for a Word comment.
func CommentLines(it check.Item) []string {
	return msg.CommentText(it.Severity.String(), it.Message, it.Detail)
}

type jsonReport struct {
	SchemaVersion int           `json:"schema_version"`
	File          string        `json:"file"`
	Profile       string        `json:"profile"`
	Stats         jsonStats     `json:"stats"`
	Findings      []jsonFinding `json:"findings"`
}

type jsonStats struct {
	Sections   int `json:"sections"`
	Paragraphs int `json:"paragraphs"`
	Headings   int `json:"headings"`
	Figures    int `json:"figures"`
	Tables     int `json:"tables"`
	Sources    int `json:"sources"`
	Errors     int `json:"errors"`
	Warnings   int `json:"warnings"`
}

type jsonFinding struct {
	Rule      string `json:"rule"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Paragraph int    `json:"paragraph"`
	Heading   string `json:"heading,omitempty"`
	Ordinal   int    `json:"ordinal,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Where     string `json:"where"`
}

// JSON writes the machine-readable report. Paragraph is the 0-based index
// in document order, -1 for findings about the whole document.
func JSON(w io.Writer, r check.Result) error {
	s := r.Stats
	out := jsonReport{
		SchemaVersion: SchemaVersion, File: r.File, Profile: r.Profile,
		Stats:    jsonStats{s.Sections, s.Paragraphs, s.Headings, s.Figures, s.Tables, s.Sources, s.Errors, s.Warnings},
		Findings: []jsonFinding{},
	}
	for _, it := range r.Findings {
		out.Findings = append(out.Findings, jsonFinding{
			Rule: string(it.Rule), Severity: it.Severity.String(), Message: it.Message, Detail: it.Detail,
			Paragraph: it.Loc.Para, Heading: it.Loc.Heading, Ordinal: it.Loc.Ordinal, Snippet: it.Loc.Snippet,
			Where: msg.Where(it.Loc),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}
