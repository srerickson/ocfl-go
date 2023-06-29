package logging

import (
	"os"

	"golang.org/x/exp/slog"
)

var (
	defaultOptions = slog.HandlerOptions{
		Level: slog.LevelError,
	}
	defaultHanlder = slog.NewTextHandler(os.Stderr, &defaultOptions)
	defaultLogger  = slog.New(defaultHanlder)
)

func DefaultLogger() *slog.Logger {
	return defaultLogger
}
