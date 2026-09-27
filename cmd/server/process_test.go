package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"other":   slog.LevelInfo,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := parseLogLevel(input); got != want {
				t.Fatalf("parseLogLevel(%q)=%v want %v", input, got, want)
			}
		})
	}
}

func TestConfigureProcessLoggerJSON(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	var buffer bytes.Buffer
	configureProcessLogger("info", "json", &buffer)
	logBuildInfo()
	line := strings.TrimSpace(buffer.String())
	if line == "" {
		t.Fatal("expected a startup log line")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(line), &document); err != nil {
		t.Fatalf("startup log is not JSON: %v %s", err, line)
	}
	if document["msg"] != "headless backend starting" {
		t.Fatalf("unexpected startup log: %s", line)
	}
	if document["module"] != "github.com/fastygo/backend" {
		t.Fatalf("missing module field: %s", line)
	}
	if _, ok := document["revision"]; !ok {
		t.Fatalf("missing revision field: %s", line)
	}
	secretKeys := []string{"HEADLESS_TOKEN_SECRET", "password", "secret", "dsn"}
	for _, key := range secretKeys {
		if _, found := document[key]; found {
			t.Fatalf("startup log leaked %s: %s", key, line)
		}
	}
}
