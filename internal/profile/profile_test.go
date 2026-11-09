package profile_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/format"
	"github.com/Santaniel0/normokontrol/internal/profile"
)

func TestBuiltinGOST(t *testing.T) {
	p, err := profile.Load(profile.DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	pg := p.Page
	if pg.MarginLeftMM != 30 || pg.MarginRightMM != 15 || pg.MarginTopMM != 20 || pg.MarginBottomMM != 20 {
		t.Errorf("margins = %+v", pg)
	}
	if p.Body.LineSpacing != 1.5 || p.Body.FirstLineIndentCM != 1.25 || p.Body.MinSizePt != 12 || p.Body.SizePt != 0 {
		t.Errorf("body = %+v", p.Body)
	}
	if p.Body.Align != 0 {
		t.Error("ГОСТ 7.32-2017 does not fix alignment, the default must leave it unset")
	}
	if !slices.Contains(p.Structure.Required, "ВВЕДЕНИЕ") || p.Captions.Numbering != profile.NumberingAny {
		t.Errorf("structure = %+v captions = %+v", p.Structure, p.Captions)
	}
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInheritance(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "university.toml", `
base = "gost-7.32-2017"
name = "Университет"
[body]
size_pt = 14
align = "justify"
font_required = true
[structure]
required = ["ВВЕДЕНИЕ", "ЗАКЛЮЧЕНИЕ"]
`)
	path := write(t, dir, "department.toml", `
base = "university.toml"
name = "Кафедра"
[page]
margin_right_mm = 10
`)
	p, err := profile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Кафедра" || p.Page.MarginRightMM != 10 || p.Page.MarginLeftMM != 30 {
		t.Errorf("page = %+v name = %q", p.Page, p.Name)
	}
	if p.Body.SizePt != 14 || p.Body.Align != format.AlignJustify || !p.Body.FontRequired || p.Body.LineSpacing != 1.5 {
		t.Errorf("body = %+v", p.Body)
	}
	if !slices.Equal(p.Structure.Required, []string{"ВВЕДЕНИЕ", "ЗАКЛЮЧЕНИЕ"}) {
		t.Errorf("required must be replaced, not merged: %v", p.Structure.Required)
	}
	if len(p.Structure.Order) == 0 {
		t.Error("order must be inherited")
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.toml", `base = "b.toml"`)
	write(t, dir, "b.toml", `base = "a.toml"`)
	typo := write(t, dir, "typo.toml", "base = \"gost-7.32-2017\"\n[page]\nmargin_lft_mm = 30\n")
	big := write(t, dir, "big.toml", "[body]\nline_spacing = 15\n")
	badAlign := write(t, dir, "align.toml", "[body]\nalign = \"middle\"\n")
	broken := write(t, dir, "broken.toml", "[page\n")

	var ve *profile.ValidationError
	if _, err := profile.Load(filepath.Join(dir, "a.toml")); !errors.Is(err, profile.ErrBaseCycle) {
		t.Errorf("cycle: %v", err)
	}
	if _, err := profile.Load(typo); !errors.As(err, &ve) || ve.Problem != profile.ProblemUnknownKey || ve.Key != "page.margin_lft_mm" {
		t.Errorf("typo: %v", err)
	}
	if _, err := profile.Load(big); !errors.As(err, &ve) || ve.Problem != profile.ProblemOutOfRange || ve.Key != "body.line_spacing" {
		t.Errorf("range: %v", err)
	}
	if _, err := profile.Load("gost-1917"); !errors.Is(err, profile.ErrUnknownBuiltin) {
		t.Errorf("unknown builtin: %v", err)
	}
	for _, f := range []string{badAlign, broken} {
		if _, err := profile.Load(f); err == nil {
			t.Errorf("%s: expected an error", f)
		}
	}
}
