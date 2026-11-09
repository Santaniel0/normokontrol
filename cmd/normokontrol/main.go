// Command normokontrol checks the formatting of student papers in .docx
// against ГОСТ 7.32-2017 or a university profile.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Santaniel0/normokontrol/internal/annotate"
	"github.com/Santaniel0/normokontrol/internal/batch"
	"github.com/Santaniel0/normokontrol/internal/check"
	"github.com/Santaniel0/normokontrol/internal/msg"
	"github.com/Santaniel0/normokontrol/internal/ooxml"
	"github.com/Santaniel0/normokontrol/internal/profile"
	"github.com/Santaniel0/normokontrol/internal/report"
	"github.com/Santaniel0/normokontrol/internal/rules"
)

const (
	exitOK       = 0
	exitFindings = 1
	exitFailure  = 2
)

// annotatedSuffix marks copies with comments; batch skips such files.
const annotatedSuffix = ".проверено.docx"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	e := env{stdin, stdout, stderr}
	if len(args) == 0 {
		fmt.Fprintln(stderr, msg.Usage)
		return exitFailure
	}
	switch args[0] {
	case "check":
		return e.check(ctx, args[1:])
	case "batch":
		return e.batch(ctx, args[1:])
	case "profile":
		return e.profile(args[1:])
	case "rules":
		for _, r := range rules.All() {
			fmt.Fprintf(stdout, "%-16s %s\n", r.ID(), msg.RuleTitles[string(r.ID())])
		}
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, msg.Usage)
		return exitOK
	}
	if st, err := os.Stat(args[0]); len(args) == 1 && err == nil && !st.IsDir() {
		return e.dragAndDrop(ctx, args[0])
	}
	fmt.Fprintln(stderr, msg.Usage)
	return exitFailure
}

// dragAndDrop is what runs when a file is dropped onto the .exe in
// Windows Explorer: check, annotate, and keep the console window open.
func (e env) dragAndDrop(ctx context.Context, path string) int {
	code := exitFailure
	if p, err := loadProfile(""); err != nil {
		e.fail(path, err)
	} else {
		code = e.checkOne(ctx, path, p, "text", true)
	}
	fmt.Fprint(e.stdout, "\n"+msg.PressEnter)
	_, _ = bufio.NewReader(e.stdin).ReadString('\n')
	return code
}

func (e env) check(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	prof := fs.String("profile", "", msg.FlagProfile)
	format := fs.String("format", "text", msg.FlagFormat)
	ann := fs.Bool("annotate", false, msg.FlagAnnotate)
	files, err := parseInterleaved(fs, args)
	if err != nil {
		return exitFailure
	}
	if len(files) != 1 {
		fmt.Fprintln(e.stderr, msg.ErrNeedOneFile)
		return exitFailure
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(e.stderr, msg.ErrBadFormat(*format))
		return exitFailure
	}
	p, err := loadProfile(*prof)
	if err != nil {
		e.fail(*prof, err)
		return exitFailure
	}
	return e.checkOne(ctx, files[0], p, *format, *ann)
}

func (e env) checkOne(ctx context.Context, path string, p *profile.Profile, format string, ann bool) int {
	c, err := check.File(ctx, path, p)
	if err != nil {
		e.fail(path, err)
		return exitFailure
	}
	if err := writeReport(e.stdout, c.Result, format); err != nil {
		e.fail(path, err)
		return exitFailure
	}
	if ann {
		out := annotatedPath(path, "")
		n, err := writeAnnotated(c, out)
		if err != nil {
			e.fail(path, err)
			return exitFailure
		}
		line := msg.AnnotatedLine(out, n)
		if format == "json" {
			fmt.Fprintln(e.stderr, line) // keep stdout valid JSON
		} else {
			fmt.Fprintln(e.stdout, line)
		}
	}
	if c.Result.Stats.Errors > 0 {
		return exitFindings
	}
	return exitOK
}

func writeReport(w io.Writer, r check.Result, format string) error {
	if format == "json" {
		return report.JSON(w, r)
	}
	return report.Text(w, r)
}

func annotatedPath(path, dir string) string {
	if dir == "" {
		dir = filepath.Dir(path)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return filepath.Join(dir, base+annotatedSuffix)
}

// writeAnnotated writes through a temporary file so that a failure never
// leaves a half-written .docx that Word would call corrupted.
func writeAnnotated(c *check.Checked, out string) (int, error) {
	tmp, err := os.CreateTemp(filepath.Dir(out), ".normokontrol-*.docx")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	n, err := c.Annotate(tmp, report.CommentLines)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	return n, os.Rename(tmp.Name(), out)
}

func (e env) batch(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	prof := fs.String("profile", "", msg.FlagProfile)
	format := fs.String("format", "text", msg.FlagFormat)
	ann := fs.Bool("annotate", false, msg.FlagAnnotate)
	out := fs.String("out", "", msg.FlagOut)
	jobs := fs.Int("jobs", runtime.NumCPU(), msg.FlagJobs)
	inputs, err := parseInterleaved(fs, args)
	if err != nil {
		return exitFailure
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(e.stderr, msg.ErrBadFormat(*format))
		return exitFailure
	}
	p, err := loadProfile(*prof)
	if err != nil {
		e.fail(*prof, err)
		return exitFailure
	}
	paths, err := collect(inputs)
	if err != nil {
		e.fail("", err)
		return exitFailure
	}
	if len(paths) == 0 {
		fmt.Fprintln(e.stderr, msg.BatchNoFiles)
		return exitFailure
	}
	if *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			e.fail(*out, err)
			return exitFailure
		}
	}
	fc := &fileChecker{profile: p, out: *out, format: *format, annotate: *ann}
	fmt.Fprintln(e.stdout, msg.BatchHeader())
	clean, failed, withErrors := 0, 0, 0
	batch.Run(ctx, paths, *jobs, fc, func(o batch.Outcome) {
		name := filepath.Base(o.Path)
		if o.Err != nil {
			failed++
			fmt.Fprintln(e.stdout, msg.BatchFailed(name, explain(o.Err)))
			return
		}
		if o.Result.Stats.Errors == 0 {
			clean++
		} else {
			withErrors++
		}
		fmt.Fprintln(e.stdout, msg.BatchRow(name, o.Result.Stats.Errors, o.Result.Stats.Warnings, fc.reportPath(o.Path)))
	})
	fmt.Fprintln(e.stdout, msg.BatchTotal(len(paths), clean, failed))
	switch {
	case failed > 0:
		return exitFailure
	case withErrors > 0:
		return exitFindings
	}
	return exitOK
}

type fileChecker struct {
	profile  *profile.Profile
	out      string
	format   string
	annotate bool
}

func (f *fileChecker) reportPath(path string) string {
	if f.out == "" {
		return "—"
	}
	ext := ".txt"
	if f.format == "json" {
		ext = ".json"
	}
	return filepath.Join(f.out, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+ext)
}

// CheckFile implements batch.Checker.
func (f *fileChecker) CheckFile(ctx context.Context, path string) (check.Result, error) {
	c, err := check.File(ctx, path, f.profile)
	if err != nil {
		return check.Result{}, err
	}
	if f.out != "" {
		rf, err := os.Create(f.reportPath(path))
		if err != nil {
			return c.Result, err
		}
		err = writeReport(rf, c.Result, f.format)
		if cerr := rf.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return c.Result, err
		}
	}
	if f.annotate {
		if _, err := writeAnnotated(c, annotatedPath(path, f.out)); err != nil {
			return c.Result, err
		}
	}
	return c.Result, nil
}

// collect expands directories into .docx files, skipping Word lock files
// ("~$...") and our own annotated copies.
func collect(inputs []string) ([]string, error) {
	var out []string
	for _, in := range inputs {
		st, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			out = append(out, in)
			continue
		}
		err = filepath.WalkDir(in, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() || !strings.EqualFold(filepath.Ext(name), ".docx") ||
				strings.HasPrefix(name, "~$") || strings.HasSuffix(name, annotatedSuffix) {
				return nil
			}
			out = append(out, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (e env) profile(args []string) int {
	name := profile.DefaultName
	if len(args) > 0 {
		name = args[0]
	}
	src, err := profile.BuiltinSource(name)
	if err != nil {
		e.fail(name, err)
		fmt.Fprintln(e.stderr, msg.BuiltinProfiles(profile.BuiltinNames()))
		return exitFailure
	}
	_, _ = e.stdout.Write(src)
	return exitOK
}

// loadProfile picks --profile, then normokontrol.toml next to the
// executable (so a department can hand out the .exe with its rules), then
// the built-in default.
func loadProfile(ref string) (*profile.Profile, error) {
	if ref == "" {
		ref = profile.DefaultName
		if exe, err := os.Executable(); err == nil {
			local := filepath.Join(filepath.Dir(exe), "normokontrol.toml")
			if _, err := os.Stat(local); err == nil {
				ref = local
			}
		}
	}
	p, err := profile.Load(ref)
	if err != nil {
		return nil, err
	}
	if _, err := rules.Enabled(p); err != nil {
		return nil, err
	}
	return p, nil
}

// parseInterleaved lets flags follow file names: "check vkr.docx --annotate".
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func (e env) fail(subject string, err error) {
	text := explain(err)
	if subject != "" {
		text = msg.FileError(subject, text)
	}
	fmt.Fprintln(e.stderr, text)
}

// explain turns known errors into Russian sentences; the wording is in msg.
func explain(err error) string {
	var ve *profile.ValidationError
	var ue *rules.UnknownRuleError
	switch {
	case errors.Is(err, ooxml.ErrLegacyDOC):
		return msg.ErrLegacyDOC
	case errors.Is(err, ooxml.ErrNotDOCX):
		return msg.ErrNotDOCX
	case errors.Is(err, ooxml.ErrTooLarge), errors.Is(err, check.ErrFileTooLarge):
		return msg.ErrTooLarge
	case errors.Is(err, ooxml.ErrMalformedXML):
		return msg.ErrMalformed
	case errors.As(err, &ve):
		return msg.ErrProfileKey(ve.File, ve.Key, ve.Problem == profile.ProblemOutOfRange)
	case errors.Is(err, profile.ErrUnknownBuiltin):
		return msg.ErrUnknownProfile
	case errors.Is(err, profile.ErrBaseCycle):
		return msg.ErrBaseCycle
	case errors.As(err, &ue):
		return msg.ErrUnknownRule(ue.ID)
	case errors.Is(err, annotate.ErrUnsupported):
		return msg.ErrAnnotate
	case errors.Is(err, fs.ErrNotExist):
		return msg.ErrNotFound
	}
	return err.Error()
}
