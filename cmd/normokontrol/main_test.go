package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Santaniel0/normokontrol/internal/msg"
)

func copySample(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCheckExitCodes(t *testing.T) {
	dir := t.TempDir()
	good := copySample(t, dir, "vkr_good.docx")
	bad := copySample(t, dir, "vkr_bad.docx")
	legacy := filepath.Join(dir, "old.docx")
	if err := os.WriteFile(legacy, append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 600)...), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		args    []string
		code    int
		stdout  string
		stderr  string
		created string
	}{
		{"clean document", []string{"check", good}, exitOK, msg.NoFindings, "", ""},
		{"errors found, flag after file", []string{"check", bad, "--annotate"}, exitFindings, "body.spacing", "", "vkr_bad" + annotatedSuffix},
		{"json keeps stdout parseable", []string{"check", "--format", "json", good}, exitOK, `"schema_version": 1`, "", ""},
		{"legacy doc", []string{"check", legacy}, exitFailure, "", msg.ErrLegacyDOC, ""},
		{"missing file", []string{"check", filepath.Join(dir, "nope.docx")}, exitFailure, "", msg.ErrNotFound, ""},
		{"bad format", []string{"check", "--format", "pdf", good}, exitFailure, "", "неизвестный формат", ""},
		{"unknown profile", []string{"check", "--profile", "gost-1917", good}, exitFailure, "", msg.ErrUnknownProfile, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, "", tt.args...)
			if code != tt.code || !strings.Contains(out, tt.stdout) || !strings.Contains(errOut, tt.stderr) {
				t.Fatalf("code %d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
			}
			if tt.created != "" {
				if _, err := os.Stat(filepath.Join(dir, tt.created)); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestDragAndDropWaitsForEnter(t *testing.T) {
	dir := t.TempDir()
	bad := copySample(t, dir, "vkr_bad.docx")
	code, out, _ := runCLI(t, "\n", bad)
	if code != exitFindings || !strings.HasSuffix(out, msg.PressEnter) || !strings.Contains(out, "Копия с примечаниями") {
		t.Fatalf("code %d, stdout:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "vkr_bad"+annotatedSuffix)); err != nil {
		t.Error(err)
	}
}

func TestBatch(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "reports")
	copySample(t, in, "vkr_good.docx")
	copySample(t, in, "vkr_bad.docx")
	for _, skip := range []string{"~$vkr_bad.docx", "vkr_good" + annotatedSuffix, "notes.txt"} {
		if err := os.WriteFile(filepath.Join(in, skip), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runCLI(t, "", "batch", "--out", out, "--jobs", "2", "--annotate", in)
	if code != exitFindings {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "vkr_bad.docx") || !strings.HasPrefix(lines[2], "vkr_good.docx") {
		t.Fatalf("summary:\n%s", stdout)
	}
	for _, f := range []string{"vkr_bad.txt", "vkr_good.txt", "vkr_bad" + annotatedSuffix} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Error(err)
		}
	}
}

func TestProfileAndRules(t *testing.T) {
	code, out, _ := runCLI(t, "", "profile")
	if code != exitOK || !strings.Contains(out, "margin_left_mm = 30") {
		t.Errorf("profile: %d\n%s", code, out)
	}
	code, out, _ = runCLI(t, "", "rules")
	if code != exitOK || !strings.Contains(out, "text.homoglyph") {
		t.Errorf("rules: %d\n%s", code, out)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "p.toml")
	if err := os.WriteFile(bad, []byte("base = \"gost-7.32-2017\"\n[rules]\ndisable = [\"body.fonts\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "", "check", "--profile", bad, copySample(t, dir, "vkr_good.docx"))
	if code != exitFailure || !strings.Contains(errOut, "неизвестное правило") {
		t.Errorf("unknown rule: %d %s", code, errOut)
	}
}
