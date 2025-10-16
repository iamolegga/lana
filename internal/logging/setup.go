package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/phsym/console-slog"
)

func Setup(level, format string, env string) {
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(level)); err != nil {
		panic(err)
	}

	var handler slog.Handler
	var opts *slog.HandlerOptions = &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case "time":
				return slog.Attr{
					Key: a.Key,
					Value: slog.StringValue(
						a.Value.Time().UTC().Format("2006-01-02T15:04:05.999Z"),
					),
				}
			case slog.SourceKey:
				source := a.Value.Any().(*slog.Source)
				fullPath := source.File

				if idx := strings.Index(fullPath, "/go/pkg/mod/"); idx != -1 {
					modPath := fullPath[idx+len("/go/pkg/mod/"):]
					if atIdx := strings.Index(modPath, "@"); atIdx != -1 {
						if slashIdx := strings.Index(modPath[atIdx:], "/"); slashIdx != -1 {
							modPath = modPath[:atIdx] + modPath[atIdx+slashIdx:]
						}
					}
					source.File = modPath
				} else if idx := strings.Index(fullPath, "/build/"); idx != -1 {
					source.File = fullPath[idx+len("/build/"):]
				} else {
					parts := []string{"/lana/", "/projects/lana/"}
					cleaned := false
					for _, marker := range parts {
						if idx := strings.LastIndex(fullPath, marker); idx != -1 {
							source.File = fullPath[idx+len(marker):]
							cleaned = true
							break
						}
					}
					if !cleaned {
						source.File = filepath.Base(fullPath)
					}
				}
				return slog.Any(slog.SourceKey, source)
			default:
				return a
			}
		},
	}

	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		if env == "development" {
			handler = console.NewHandler(os.Stdout, &console.HandlerOptions{
				AddSource:  opts.AddSource,
				Level:      opts.Level,
				TimeFormat: "15:04:05",
			})
		} else {
			handler = slog.NewTextHandler(os.Stdout, opts)
		}
	default:
		panic("invalid format")
	}

	slog.SetDefault(slog.New(handler))
}
