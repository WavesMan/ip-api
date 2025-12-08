package logger

import (
  "log/slog"
  "os"
  "strings"
)

var defaultLogger *slog.Logger

func Setup() *slog.Logger {
  lvl := slog.LevelInfo
  switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
  case "debug":
    lvl = slog.LevelDebug
  case "warn":
    lvl = slog.LevelWarn
  case "error":
    lvl = slog.LevelError
  }
  format := strings.ToLower(os.Getenv("LOG_FORMAT"))
  var h slog.Handler
  if format == "json" {
    h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
  } else {
    h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
  }
  defaultLogger = slog.New(h)
  return defaultLogger
}

func L() *slog.Logger {
  if defaultLogger == nil {
    return Setup()
  }
  return defaultLogger
}

