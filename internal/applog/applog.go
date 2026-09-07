package applog

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	mu     sync.RWMutex
	logger = newLogger(os.Stderr)
)

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceAttr}))
}

func replaceAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		attr.Key = "time"
	}
	if attr.Key == slog.LevelKey {
		attr.Key = "level"
		attr.Value = slog.StringValue(strings.ToLower(attr.Value.String()))
	}
	if attr.Key == slog.MessageKey {
		attr.Key = "event"
	}
	return attr
}

func SetOutput(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	mu.Lock()
	logger = newLogger(w)
	mu.Unlock()
}

func Info(module, event string, args ...any)  { current().Info(event, fields(module, args...)...) }
func Warn(module, event string, args ...any)  { current().Warn(event, fields(module, args...)...) }
func Error(module, event string, args ...any) { current().Error(event, fields(module, args...)...) }
func Debug(module, event string, args ...any) { current().Debug(event, fields(module, args...)...) }

func Fatal(module, event string, args ...any) {
	Error(module, event, args...)
	os.Exit(1)
}

func FatalError(module, event string, err error, args ...any) {
	Error(module, event, append(args, "error", err)...)
	os.Exit(1)
}

func ErrorValue(err error) any {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func StringValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprint(value)
}

func current() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

func fields(module string, args ...any) []any {
	result := make([]any, 0, len(args)+2)
	result = append(result, "module", module)
	result = append(result, args...)
	return result
}

func ConfigureStandardLog(module string) {
	log.SetFlags(0)
	log.SetOutput(&standardWriter{module: module})
}

type standardWriter struct{ module string }

func (w *standardWriter) Write(p []byte) (int, error) {
	Info(w.module, "legacy_log", "message", strings.TrimSpace(string(p)))
	return len(p), nil
}
