//	if err := log.Init(cfg.LogLevel, cfg.LogFile); err != nil {
//	    return err
//	}
//
// defer log.Close()
// slog.Info("provider loaded", "name", p.FriendlyName())
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var logFile *os.File

func Init(level, path string) error {
	var w io.Writer = io.Discard
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		logFile = f
		w = f
	}

	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
	return nil
}

func Close() {
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
