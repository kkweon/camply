package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

const LevelCamply = slog.LevelInfo + 1

var logLevel = new(slog.LevelVar)

// DebugMode is bound to the global --debug flag
var DebugMode bool

func init() {
	h := &CamplyHandler{
		Level: logLevel,
	}
	slog.SetDefault(slog.New(h))
}

// Setup configure the logger based on flags
func Setup() {
	if DebugMode {
		logLevel.Set(slog.LevelDebug)
	} else {
		logLevel.Set(slog.LevelInfo)
	}
}

// CamplyHandler is a custom slog handler to match Python camply's output format
type CamplyHandler struct {
	Level slog.Leveler
}

func (h *CamplyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.Level.Level()
}

func (h *CamplyHandler) Handle(_ context.Context, r slog.Record) error {
	levelStr := r.Level.String()
	switch r.Level {
	case slog.LevelInfo:
		levelStr = "INFO    "
	case slog.LevelError:
		levelStr = "ERROR   "
	case slog.LevelDebug:
		levelStr = "DEBUG   "
	case LevelCamply:
		levelStr = "CAMPLY  "
	}

	out := os.Stdout
	if r.Level == slog.LevelError {
		out = os.Stderr
	}

	_, _ = fmt.Fprintf(out, "[%s] %s %s\n", r.Time.Format("2006-01-02 15:04:05"), levelStr, r.Message)
	return nil
}

func (h *CamplyHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *CamplyHandler) WithGroup(name string) slog.Handler       { return h }

// Info logs an informational message
func Info(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), slog.LevelInfo, fmt.Sprintf(format, a...))
}

// Error logs an error message
func Error(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), slog.LevelError, fmt.Sprintf(format, a...))
}

// Camply logs a generic camply message (like startup/exit)
func Camply(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), LevelCamply, fmt.Sprintf(format, a...))
}

// Debug logs a debug message
func Debug(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), slog.LevelDebug, fmt.Sprintf(format, a...))
}
