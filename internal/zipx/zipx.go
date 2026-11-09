// Package zipx writes zip entries the way strict OOXML readers expect them.
//
// archive/zip's CreateHeader streams data and therefore always sets the
// data-descriptor flag (0x8): the local file header carries zero CRC and
// sizes, and the real values follow the data. That is legal zip, but
// LibreOffice refuses such a package, and for a stored entry a reader that
// walks local headers cannot even tell where the data ends. Here every entry
// is compressed in memory first and written with CreateRaw, so the local
// header holds the CRC-32 and both sizes and no descriptor follows.
package zipx

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

const (
	flagDataDescriptor = 0x8
	zipVersion20       = 20
)

// Write adds an entry with method zip.Store or zip.Deflate.
func Write(zw *zip.Writer, name string, method uint16, mod time.Time, data []byte) error {
	body := data
	switch method {
	case zip.Store:
	case zip.Deflate:
		var buf bytes.Buffer
		fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
		if err := fw.Close(); err != nil {
			return err
		}
		body = buf.Bytes()
	default:
		return fmt.Errorf("zip entry %s: unsupported method %d", name, method)
	}
	hdr := &zip.FileHeader{
		Name:               name,
		Method:             method,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(body)),
		UncompressedSize64: uint64(len(data)),
		// CreateRaw, unlike CreateHeader, leaves the version fields at 0,
		// and .NET's System.IO.Packaging rejects such entries as
		// "unsupported data structures". 2.0 is what deflate requires.
		CreatorVersion: zipVersion20,
		ReaderVersion:  zipVersion20,
	}
	// CreateRaw does not derive the MS-DOS time from Modified either.
	//lint:ignore SA1019 the raw header fields are the only way to set the time for CreateRaw.
	hdr.ModifiedDate, hdr.ModifiedTime = dosTime(mod)
	w, err := zw.CreateRaw(hdr)
	if err != nil {
		return fmt.Errorf("zip entry %s: %w", name, err)
	}
	_, err = w.Write(body)
	return err
}

// Copy copies an entry without recompressing it. The CRC and sizes come
// from the central directory, so a data descriptor in the source is dropped
// rather than carried over.
func Copy(zw *zip.Writer, f *zip.File) error {
	hdr := f.FileHeader
	hdr.Flags &^= flagDataDescriptor
	w, err := zw.CreateRaw(&hdr)
	if err != nil {
		return fmt.Errorf("zip entry %s: %w", f.Name, err)
	}
	r, err := f.OpenRaw()
	if err != nil {
		return fmt.Errorf("zip entry %s: %w", f.Name, err)
	}
	_, err = io.Copy(w, r)
	return err
}

func dosTime(t time.Time) (date, clock uint16) {
	if t.Year() < 1980 {
		t = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	date = uint16(t.Day() + int(t.Month())<<5 + (t.Year()-1980)<<9)
	clock = uint16(t.Second()/2 + t.Minute()<<5 + t.Hour()<<11)
	return date, clock
}

// Verify walks the local file headers of an archive written by this
// package and checks that none uses a data descriptor, that the version
// needed to extract is set, and that every local header agrees with the
// central directory on CRC and sizes. Stored entries must have equal
// compressed and uncompressed sizes.
func Verify(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	le16 := func(off int) int { return int(binary.LittleEndian.Uint16(data[off:])) }
	le32 := func(off int) uint32 { return binary.LittleEndian.Uint32(data[off:]) }
	off := 0
	for _, f := range zr.File {
		if off+30 > len(data) || le32(off) != 0x04034b50 {
			return fmt.Errorf("%s: no local file header at offset %d", f.Name, off)
		}
		version, flags, method := le16(off+4), le16(off+6), le16(off+8)
		crc, csize, usize := le32(off+14), le32(off+18), le32(off+22)
		nameLen, extraLen := le16(off+26), le16(off+28)
		if off+30+nameLen > len(data) || string(data[off+30:off+30+nameLen]) != f.Name {
			return fmt.Errorf("%s: local header name does not match the central directory", f.Name)
		}
		switch {
		case version < 10 || f.ReaderVersion < 10:
			return fmt.Errorf("%s: version needed to extract is %d", f.Name, version)
		case flags&flagDataDescriptor != 0:
			return fmt.Errorf("%s: data descriptor flag is set", f.Name)
		case crc != f.CRC32 || uint64(csize) != f.CompressedSize64 || uint64(usize) != f.UncompressedSize64:
			return fmt.Errorf("%s: local header crc/sizes %08x/%d/%d differ from central directory %08x/%d/%d",
				f.Name, crc, csize, usize, f.CRC32, f.CompressedSize64, f.UncompressedSize64)
		case method == int(zip.Store) && csize != usize:
			return fmt.Errorf("%s: stored entry with compressed size %d != size %d", f.Name, csize, usize)
		}
		off += 30 + nameLen + extraLen + int(csize)
	}
	return nil
}
