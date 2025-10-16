package server

import (
	"log/slog"
	"net/http"
	"path/filepath"
)

func (s *Server) handlerRoot(w http.ResponseWriter, r *http.Request) {
	hostConfig, exists := s.hosts[r.Host]
	if !exists {
		http.Error(w, "Unknown host", http.StatusBadRequest)
		return
	}

	path := filepath.Join(hostConfig.loginDir, r.URL.Path)

	info, err := http.Dir(hostConfig.loginDir).Open(r.URL.Path)
	defer func() {
		if info != nil {
			_ = info.Close()
		}
	}()
	if err != nil {
		slog.Warn("failed to serve", "path", path, "error", err)
		http.NotFound(w, r)
		return
	}

	stat, err := info.Stat()
	if err != nil {
		slog.Warn("failed to stat login page", "path", path, "error", err)
		http.NotFound(w, r)
		return
	}

	if stat.IsDir() {
		path = filepath.Join(path, "index.html")
	}

	slog.Debug("serving file", "path", path)
	http.ServeFile(w, r, path)
}
