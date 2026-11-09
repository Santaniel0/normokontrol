package zipx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

var mod = time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)

func entries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		out[f.Name] = string(b)
	}
	return out
}

func TestWriteHasSizesInLocalHeader(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	xml := strings.Repeat("<w:p/>", 200)
	if err := Write(zw, "word/document.xml", zip.Deflate, mod, []byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := Write(zw, "word/media/image1.png", zip.Store, mod, []byte("\x89PNG fake")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	got := entries(t, buf.Bytes())
	if got["word/document.xml"] != xml || got["word/media/image1.png"] != "\x89PNG fake" {
		t.Errorf("content changed: %q", got)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if m := zr.File[0].Modified; !m.Equal(mod.Truncate(2 * time.Second)) {
		t.Errorf("modified = %v, want %v", m, mod)
	}
}

// archive/zip's own writer sets the descriptor flag; Copy must drop it.
func TestCopyDropsDataDescriptor(t *testing.T) {
	var src bytes.Buffer
	zw := zip.NewWriter(&src)
	for _, m := range []uint16{zip.Store, zip.Deflate} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: "part" + string(rune('0'+m)), Method: m})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("данные части")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(src.Bytes()); err == nil {
		t.Fatal("Verify accepted an archive with data descriptors")
	}

	zr, err := zip.NewReader(bytes.NewReader(src.Bytes()), int64(src.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var dst bytes.Buffer
	zw = zip.NewWriter(&dst)
	for _, f := range zr.File {
		if err := Copy(zw, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(dst.Bytes()); err != nil {
		t.Fatal(err)
	}
	for name, body := range entries(t, dst.Bytes()) {
		if body != "данные части" {
			t.Errorf("%s = %q", name, body)
		}
	}
}

func TestWriteRejectsUnknownMethod(t *testing.T) {
	if err := Write(zip.NewWriter(io.Discard), "x", 99, mod, nil); err == nil {
		t.Error("expected an error")
	}
}
