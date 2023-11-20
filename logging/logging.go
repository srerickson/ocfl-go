package logging

import (
	"context"
	"log/slog"
	"os"
)

var (
	defaultLevel   slog.LevelVar
	defaultHanlder = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &defaultLevel,
	})
	defaultLogger  = slog.New(defaultHanlder)
	disabledLogger = slog.New(&disabledHandler{})
)

// disabledHandler is a slog.Handler that is disabled for all levels
type disabledHandler struct{}

func (d *disabledHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (d *disabledHandler) Handle(context.Context, slog.Record) error { return nil }
func (d *disabledHandler) WithAttrs([]slog.Attr) slog.Handler        { return d }
func (d *disabledHandler) WithGroup(string) slog.Handler             { return d }

// DefaultLogger returns the default (module-specific) logger.
func DefaultLogger() *slog.Logger {
	return defaultLogger
}

// SetDefaultLevel sets the logging level for the module's default logger.
func SetDefaultLevel(l slog.Level) {
	defaultLevel.Set(l)
}

// DisabledLogger returns a logger that is disabled for all logging levels.
func DisabledLogger() *slog.Logger {
	return disabledLogger
}
