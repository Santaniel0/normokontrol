package check_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/check"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/profile"
	"github.com/Santaniel0/normokontrol/internal/report"
	"github.com/Santaniel0/normokontrol/internal/sample"
	"github.com/Santaniel0/normokontrol/internal/zipx"
)

var update = flag.Bool("update", false, "rewrite golden reports in testdata")

const testdata = "../../testdata"

func gost(t *testing.T) *profile.Profile {
	t.Helper()
	p, err := profile.Load(profile.DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The committed samples must be exactly what the generator produces, so the
// README output can always be reproduced.
func TestSamplesAreUpToDate(t *testing.T) {
	for name, build := range map[string]func() ([]byte, error){"vkr_good.docx": sample.Good, "vkr_bad.docx": sample.Bad} {
		want, err := build()
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(testdata, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run go run ./cmd/mksample", name)
		}
		if err := zipx.Verify(got); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestGoodSampleIsClean(t *testing.T) {
	c, err := check.File(context.Background(), filepath.Join(testdata, "vkr_good.docx"), gost(t))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(c.Result.Findings); n != 0 {
		var buf bytes.Buffer
		_ = report.Text(&buf, c.Result)
		t.Fatalf("%d findings in the clean sample:\n%s", n, buf.String())
	}
}

func TestBadSampleReport(t *testing.T) {
	c, err := check.File(context.Background(), filepath.Join(testdata, "vkr_bad.docx"), gost(t))
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if err := report.Text(&text, c.Result); err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join(testdata, "vkr_bad.report.txt")
	if *update {
		if err := os.WriteFile(golden, text.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(text.Bytes(), want) {
		t.Errorf("report differs from %s (rerun with -update if the change is intended):\n%s", golden, text.String())
	}

	var js bytes.Buffer
	if err := report.JSON(&js, c.Result); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		SchemaVersion int `json:"schema_version"`
		Stats         struct {
			Errors int `json:"errors"`
		} `json:"stats"`
		Findings []struct {
			Rule string `json:"rule"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(js.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.SchemaVersion != report.SchemaVersion || len(parsed.Findings) != len(c.Result.Findings) || parsed.Stats.Errors != c.Result.Stats.Errors {
		t.Errorf("json = %+v", parsed)
	}

	var out bytes.Buffer
	n, err := c.Annotate(&out, report.CommentLines)
	if err != nil {
		t.Fatal(err)
	}
	if err := zipx.Verify(out.Bytes()); err != nil {
		t.Fatal(err)
	}
	pkg, err := ooxml.Open(bytes.NewReader(out.Bytes()), int64(out.Len()), ooxml.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ooxml.Load(pkg)
	if err != nil {
		t.Fatal(err)
	}
	comments, err := pkg.ReadPart("word/comments.xml")
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(comments, []byte("<w:comment ")); got != n || n == 0 {
		t.Errorf("comments.xml has %d comments, Annotate reported %d", got, n)
	}
	if got := bytes.Count(doc.XML, []byte("<w:commentReference ")); got != n {
		t.Errorf("document.xml has %d references, want %d", got, n)
	}
}

func TestRejectsLegacyDoc(t *testing.T) {
	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 1024)...)
	if _, err := check.Bytes(context.Background(), "old.doc", ole, gost(t)); !errors.Is(err, ooxml.ErrLegacyDOC) {
		t.Errorf("err = %v", err)
	}
}
