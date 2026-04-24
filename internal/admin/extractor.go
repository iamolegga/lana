package admin

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafeEntry is returned when a ZIP entry has a path that escapes the
// destination directory, is absolute, or is a symlink.
var ErrUnsafeEntry = errors.New("unsafe zip entry")

// ExtractZip extracts the ZIP at zipPath into destDir. destDir must already
// exist. Each entry's path is validated to sit under destDir (no ZipSlip),
// and any symlink entries are rejected.
func ExtractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}

	for _, f := range r.File {
		if err := extractEntry(f, absDest); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(f *zip.File, absDest string) error {
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symlink", ErrUnsafeEntry, f.Name)
	}

	// Reject absolute paths and anything that, after cleaning, escapes destDir.
	if filepath.IsAbs(f.Name) || strings.HasPrefix(f.Name, "/") {
		return fmt.Errorf("%w: %s is absolute", ErrUnsafeEntry, f.Name)
	}
	target := filepath.Join(absDest, f.Name)
	if !strings.HasPrefix(target, absDest+string(os.PathSeparator)) && target != absDest {
		return fmt.Errorf("%w: %s escapes destination", ErrUnsafeEntry, f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", f.Name, err)
	}

	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("open entry %s: %w", f.Name, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", f.Name, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy %s: %w", f.Name, err)
	}
	return nil
}
