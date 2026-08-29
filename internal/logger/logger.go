package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

const LevelCamply = slog.LevelInfo + 1

var logLevel = new(slog.LevelVar)

// DebugMode is bound to the global --debug flag
var DebugMode bool

// Output streams. Results are written at INFO, so they must stay on stdout and
// warnings must not join them there — a caller piping stdout should receive
// results only. Guarded by a mutex because providers log from goroutines.
var (
	outMu     sync.Mutex
	outWriter io.Writer = os.Stdout
	errWriter io.Writer = os.Stderr
)

// SetOutput redirects the logger's streams. Tests need this: the handler writes
// straight to the process streams, bypassing cobra's writers, so without it
// nothing logged during a command can be captured.
func SetOutput(out, errOut io.Writer) {
	outMu.Lock()
	defer outMu.Unlock()
	outWriter, errWriter = out, errOut
}

// ResetOutput restores the process streams.
func ResetOutput() { SetOutput(os.Stdout, os.Stderr) }

// write emits one record while holding outMu.
//
// slog may call Handle from several goroutines at once, so the lock has to span
// the write itself, not just the writer lookup. Releasing it first let two
// records interleave in the same writer — a real data race, not a theoretical
// one: usedirect logs from a pool of five goroutines.
func write(toErr bool, format string, a ...interface{}) {
	outMu.Lock()
	defer outMu.Unlock()
	w := outWriter
	if toErr {
		w = errWriter
	}
	_, _ = fmt.Fprintf(w, format, a...)
}

// SetDebug sets the debug level without callers reaching into package state.
func SetDebug(on bool) {
	DebugMode = on
	Setup()
}

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
	case slog.LevelWarn:
		levelStr = "WARNING "
	case LevelCamply:
		levelStr = "CAMPLY  "
	}

	toErr := r.Level == slog.LevelError || r.Level == slog.LevelWarn
	write(toErr, "[%s] %s %s\n", r.Time.Format("2006-01-02 15:04:05"), levelStr, r.Message)
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

// Warn logs a warning. It goes to stderr so it never contaminates the results
// stream, which is written at INFO on stdout.
func Warn(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), slog.LevelWarn, fmt.Sprintf(format, a...))
}

// Camply logs a generic camply message (like startup/exit)
func Camply(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), LevelCamply, fmt.Sprintf(format, a...))
}

// Debug logs a debug message
func Debug(format string, a ...interface{}) {
	slog.Default().Log(context.Background(), slog.LevelDebug, fmt.Sprintf(format, a...))
}
