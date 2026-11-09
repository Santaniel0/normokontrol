// Package profile loads formatting requirements from TOML files. A profile
// can inherit from a built-in one or from another file via "base".
package profile

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Santaniel0/normokontrol/internal/format"
)

// DefaultName is the built-in profile used when none is given.
const DefaultName = "gost-7.32-2017"

//go:embed builtin/*.toml
var builtinFS embed.FS

// Millimeters, Centimeters and Points keep profile units apart from Word's.
type (
	Millimeters float64
	Centimeters float64
	Points      float64
)

// NumberingMode is how figures and tables must be numbered.
type NumberingMode int

const (
	NumberingAny        NumberingMode = iota // either scheme, but one per document
	NumberingContinuous                      // 1, 2, 3
	NumberingBySection                       // 2.1, 2.2, 3.1
)

func (m NumberingMode) String() string {
	switch m {
	case NumberingContinuous:
		return "continuous"
	case NumberingBySection:
		return "by-section"
	}
	return "any"
}

// UnmarshalText parses the TOML value.
func (m *NumberingMode) UnmarshalText(b []byte) error {
	switch string(b) {
	case "any", "":
		*m = NumberingAny
	case "continuous":
		*m = NumberingContinuous
	case "by-section":
		*m = NumberingBySection
	default:
		return fmt.Errorf("unknown numbering %q (want any, continuous or by-section)", b)
	}
	return nil
}

// Profile is a set of requirements. Zero values mean "do not check".
type Profile struct {
	Name      string    `toml:"name"`
	Base      string    `toml:"base"`
	Page      Page      `toml:"page"`
	Body      Body      `toml:"body"`
	Structure Structure `toml:"structure"`
	Captions  Captions  `toml:"captions"`
	Rules     Rules     `toml:"rules"`
}

// Page holds page size and margins.
type Page struct {
	WidthMM        Millimeters `toml:"width_mm"`
	HeightMM       Millimeters `toml:"height_mm"`
	MarginLeftMM   Millimeters `toml:"margin_left_mm"`
	MarginRightMM  Millimeters `toml:"margin_right_mm"`
	MarginTopMM    Millimeters `toml:"margin_top_mm"`
	MarginBottomMM Millimeters `toml:"margin_bottom_mm"`
	ToleranceMM    Millimeters `toml:"tolerance_mm"`
}

// Body holds requirements for the main text.
type Body struct {
	Font              string           `toml:"font"`
	FontRequired      bool             `toml:"font_required"`
	SizePt            Points           `toml:"size_pt"`
	MinSizePt         Points           `toml:"min_size_pt"`
	LineSpacing       float64          `toml:"line_spacing"`
	FirstLineIndentCM Centimeters      `toml:"first_line_indent_cm"`
	IndentToleranceCM Centimeters      `toml:"indent_tolerance_cm"`
	Align             format.Alignment `toml:"align"`
}

// Structure lists structural element headings.
type Structure struct {
	Order        []string `toml:"order"`
	Required     []string `toml:"required"`
	Bibliography []string `toml:"bibliography"`
}

// Captions holds caption numbering requirements.
type Captions struct {
	Numbering NumberingMode `toml:"numbering"`
}

// Rules switches rules off by id.
type Rules struct {
	Disable []string `toml:"disable"`
}

var (
	// ErrUnknownBuiltin means a base or profile name is neither built in nor a file.
	ErrUnknownBuiltin = errors.New("unknown profile")
	// ErrBaseCycle means profiles inherit from each other in a loop.
	ErrBaseCycle = errors.New("profile inheritance cycle")
)

// Problem classifies a validation failure.
type Problem int

const (
	ProblemUnknownKey Problem = iota + 1
	ProblemOutOfRange
)

// ValidationError points at the offending key so a non-programmer can find it.
type ValidationError struct {
	File    string
	Key     string
	Problem Problem
}

func (e *ValidationError) Error() string {
	what := "unknown key"
	if e.Problem == ProblemOutOfRange {
		what = "value out of range"
	}
	return fmt.Sprintf("%s: %s: %s", e.File, e.Key, what)
}

// BuiltinNames lists embedded profiles.
func BuiltinNames() []string {
	entries, _ := fs.ReadDir(builtinFS, "builtin")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
	}
	sort.Strings(names)
	return names
}

// BuiltinSource returns the commented TOML of a built-in profile.
func BuiltinSource(name string) ([]byte, error) {
	data, err := builtinFS.ReadFile("builtin/" + name + ".toml")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownBuiltin, name)
	}
	return data, nil
}

// Load reads a profile by built-in name or file path.
func Load(ref string) (*Profile, error) {
	src, err := resolve(ref, "")
	if err != nil {
		return nil, err
	}
	p, err := decode(src, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return p, validate(p, src.name)
}

type source struct {
	name string // built-in name or absolute path
	dir  string // directory for resolving a relative base; "" for built-ins
	data []byte
}

func resolve(ref, dir string) (source, error) {
	if data, err := BuiltinSource(ref); err == nil {
		return source{name: ref, data: data}, nil
	}
	path := ref
	if !filepath.IsAbs(path) && dir != "" {
		path = filepath.Join(dir, path)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !strings.ContainsAny(ref, `/\.`) {
		return source{}, fmt.Errorf("%w: %s", ErrUnknownBuiltin, ref)
	}
	if err != nil {
		return source{}, fmt.Errorf("read profile: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return source{}, err
	}
	return source{name: abs, dir: filepath.Dir(abs), data: data}, nil
}

const maxDepth = 8

// decode applies the base chain first and then overlays this file: keys
// present in the file replace inherited values, absent keys keep them.
func decode(src source, seen map[string]bool) (*Profile, error) {
	if seen[src.name] || len(seen) >= maxDepth {
		return nil, fmt.Errorf("%w at %s", ErrBaseCycle, src.name)
	}
	seen[src.name] = true
	var head struct {
		Base string `toml:"base"`
	}
	if _, err := toml.Decode(string(src.data), &head); err != nil {
		return nil, fmt.Errorf("%s: %w", src.name, err)
	}
	p := &Profile{}
	if head.Base != "" {
		parent, err := resolve(head.Base, src.dir)
		if err != nil {
			return nil, fmt.Errorf("%s: base: %w", src.name, err)
		}
		if p, err = decode(parent, seen); err != nil {
			return nil, err
		}
	}
	md, err := toml.Decode(string(src.data), p)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src.name, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return nil, &ValidationError{File: src.name, Key: und[0].String(), Problem: ProblemUnknownKey}
	}
	return p, nil
}

func validate(p *Profile, file string) error {
	checks := []struct {
		key      string
		val, max float64
	}{
		{"page.width_mm", float64(p.Page.WidthMM), 1000},
		{"page.height_mm", float64(p.Page.HeightMM), 1000},
		{"page.margin_left_mm", float64(p.Page.MarginLeftMM), 100},
		{"page.margin_right_mm", float64(p.Page.MarginRightMM), 100},
		{"page.margin_top_mm", float64(p.Page.MarginTopMM), 100},
		{"page.margin_bottom_mm", float64(p.Page.MarginBottomMM), 100},
		{"page.tolerance_mm", float64(p.Page.ToleranceMM), 10},
		{"body.size_pt", float64(p.Body.SizePt), 72},
		{"body.min_size_pt", float64(p.Body.MinSizePt), 72},
		{"body.line_spacing", p.Body.LineSpacing, 4},
		{"body.first_line_indent_cm", float64(p.Body.FirstLineIndentCM), 5},
		{"body.indent_tolerance_cm", float64(p.Body.IndentToleranceCM), 1},
	}
	for _, c := range checks {
		if c.val < 0 || c.val > c.max {
			return &ValidationError{File: file, Key: c.key, Problem: ProblemOutOfRange}
		}
	}
	return nil
}
