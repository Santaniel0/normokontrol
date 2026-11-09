// Command mksample regenerates the synthetic theses in testdata:
//
//	go run ./cmd/mksample
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Santaniel0/normokontrol/internal/sample"
)

func main() {
	if err := run("testdata"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string) error {
	for name, build := range map[string]func() ([]byte, error){
		"vkr_good.docx": sample.Good,
		"vkr_bad.docx":  sample.Bad,
	} {
		data, err := build()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
