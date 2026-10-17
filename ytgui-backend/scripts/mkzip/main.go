// mkzip builds the user download (Fetchly.zip) from the launcher files.
//
// PowerShell's zip support records entries as made on Windows, so macOS
// drops the exec bit on start.command. archive/zip with SetMode records
// Unix permissions, which Archive Utility and unzip honour. Line endings
// are fixed per file type so the zip is the same whatever the checkout.
//
// Usage: go run ./scripts/mkzip -o out.zip -dir Fetchly file...
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	out := flag.String("o", "", "output zip path")
	dir := flag.String("dir", "Fetchly", "folder name inside the zip")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mkzip -o out.zip [-dir name] file...")
		os.Exit(2)
	}
	if err := run(*out, *dir, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "mkzip:", err)
		os.Exit(1)
	}
}

func run(out, dir string, files []string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	// A fixed timestamp keeps the zip identical across rebuilds.
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		mode := os.FileMode(0o644)
		switch strings.ToLower(filepath.Ext(name)) {
		case ".bat", ".cmd", ".txt":
			data = toCRLF(data)
		case ".command", ".sh":
			data = toLF(data)
			mode = 0o755
		}
		h := &zip.FileHeader{Name: dir + "/" + name, Method: zip.Deflate, Modified: stamp}
		h.SetMode(mode) // also marks the entry as made on Unix
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func toLF(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }

func toCRLF(b []byte) []byte { return bytes.ReplaceAll(toLF(b), []byte("\n"), []byte("\r\n")) }
