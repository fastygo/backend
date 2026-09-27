package main

import (
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
)

func configureProcessLogger(level, format string, writer io.Writer) {
	if writer == nil {
		writer = os.Stdout
	}
	options := &slog.HandlerOptions{Level: parseLogLevel(level)}
	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		handler = slog.NewJSONHandler(writer, options)
	} else {
		handler = slog.NewTextHandler(writer, options)
	}
	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func logBuildInfo() {
	goVersion := "unknown"
	module := "github.com/fastygo/backend"
	revision := "unknown"
	modified := false
	if info, ok := debug.ReadBuildInfo(); ok {
		goVersion = info.GoVersion
		if info.Main.Path != "" {
			module = info.Main.Path
		}
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			revision = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	slog.Info("headless backend starting",
		"module", module,
		"go", goVersion,
		"revision", revision,
		"modified", modified,
	)
}
