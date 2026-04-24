package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/iamolegga/lana/internal/admin"
)

func handlerAdminLoginAssetsUpload(loginDirs map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.PathValue("host")
		raw, exists := loginDirs[host]
		if !exists {
			http.Error(w, "unknown host", http.StatusNotFound)
			return
		}

		start := time.Now()
		// filepath.Clean strips any trailing slash so filepath.Dir returns the
		// true parent, not the directory itself.
		loginDir := filepath.Clean(raw)
		parent := filepath.Dir(loginDir)

		suffix, err := randomSuffix()
		if err != nil {
			slog.Error("failed to generate temp suffix", "host", host, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tmpZipPath := filepath.Join(parent, ".tmp-zip-"+suffix)
		tmpDir := filepath.Join(parent, ".tmp-"+suffix)

		// Always clean up temp artifacts on the way out.
		defer os.Remove(tmpZipPath)
		defer os.RemoveAll(tmpDir)

		if err := writeBodyToFile(r.Body, tmpZipPath); err != nil {
			slog.Error("failed to write upload body", "host", host, "error", err)
			http.Error(w, "failed to write upload", http.StatusInternalServerError)
			return
		}

		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			slog.Error("failed to create temp dir", "host", host, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := admin.ExtractZip(tmpZipPath, tmpDir); err != nil {
			slog.Error("failed to extract zip", "host", host, "error", err)
			status := http.StatusInternalServerError
			if errors.Is(err, admin.ErrUnsafeEntry) {
				status = http.StatusBadRequest
			}
			http.Error(w, "failed to extract zip: "+err.Error(), status)
			return
		}

		if err := os.RemoveAll(loginDir); err != nil {
			slog.Error("failed to remove existing login dir", "host", host, "path", loginDir, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := os.Rename(tmpDir, loginDir); err != nil {
			slog.Error("failed to swap login dir", "host", host, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		slog.Info("login assets uploaded",
			"host", host,
			"bytes", fileSize(loginDir),
			"duration", time.Since(start),
		)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

func writeBodyToFile(body io.ReadCloser, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// fileSize returns the total size of the tree rooted at path, for logging.
// Errors are swallowed — this is best-effort observability.
func fileSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
