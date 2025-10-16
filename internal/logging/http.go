package logging

import (
	"log/slog"
	"net/http"

	sloghttp "github.com/samber/slog-http"
)

func Middleware(next http.Handler) http.Handler {
	config := sloghttp.Config{
		DefaultLevel:     slog.LevelInfo,
		ServerErrorLevel: slog.LevelError,
	}

	return sloghttp.NewWithConfig(slog.Default(), config)(next)
}
