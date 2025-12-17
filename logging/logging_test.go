package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestDefaultLogger(t *testing.T) {
	t.Run("returns non-nil logger", func(t *testing.T) {
		logger := DefaultLogger()
		be.Nonzero(t, logger)
	})

	t.Run("returns same instance on multiple calls", func(t *testing.T) {
		logger1 := DefaultLogger()
		logger2 := DefaultLogger()
		be.Equal(t, logger1, logger2)
	})

	t.Run("logger is functional", func(t *testing.T) {
		logger := DefaultLogger()
		// Should not panic
		logger.Info("test message")
		logger.Debug("debug message")
		logger.Error("error message")
	})
}

func TestSetDefaultLevel(t *testing.T) {
	// Save original level to restore after test
	originalLevel := defaultLevel.Level()
	defer defaultLevel.Set(originalLevel)

	t.Run("sets level to Info", func(t *testing.T) {
		SetDefaultLevel(slog.LevelInfo)
		be.Equal(t, slog.LevelInfo, defaultLevel.Level())

		logger := DefaultLogger()
		be.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
		be.True(t, logger.Enabled(context.Background(), slog.LevelWarn))
		be.True(t, logger.Enabled(context.Background(), slog.LevelError))
	})

	t.Run("sets level to Debug", func(t *testing.T) {
		SetDefaultLevel(slog.LevelDebug)
		be.Equal(t, slog.LevelDebug, defaultLevel.Level())

		logger := DefaultLogger()
		be.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
		be.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("sets level to Warn", func(t *testing.T) {
		SetDefaultLevel(slog.LevelWarn)
		be.Equal(t, slog.LevelWarn, defaultLevel.Level())

		logger := DefaultLogger()
		be.False(t, logger.Enabled(context.Background(), slog.LevelInfo))
		be.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
		be.True(t, logger.Enabled(context.Background(), slog.LevelWarn))
		be.True(t, logger.Enabled(context.Background(), slog.LevelError))
	})

	t.Run("sets level to Error", func(t *testing.T) {
		SetDefaultLevel(slog.LevelError)
		be.Equal(t, slog.LevelError, defaultLevel.Level())

		logger := DefaultLogger()
		be.False(t, logger.Enabled(context.Background(), slog.LevelInfo))
		be.False(t, logger.Enabled(context.Background(), slog.LevelWarn))
		be.True(t, logger.Enabled(context.Background(), slog.LevelError))
	})
}

func TestDisabledLogger(t *testing.T) {
	t.Run("returns non-nil logger", func(t *testing.T) {
		logger := DisabledLogger()
		be.Nonzero(t, logger)
	})

	t.Run("returns same instance on multiple calls", func(t *testing.T) {
		logger1 := DisabledLogger()
		logger2 := DisabledLogger()
		be.Equal(t, logger1, logger2)
	})

	t.Run("logger is disabled for all levels", func(t *testing.T) {
		logger := DisabledLogger()
		ctx := context.Background()

		be.False(t, logger.Enabled(ctx, slog.LevelDebug))
		be.False(t, logger.Enabled(ctx, slog.LevelInfo))
		be.False(t, logger.Enabled(ctx, slog.LevelWarn))
		be.False(t, logger.Enabled(ctx, slog.LevelError))
		be.False(t, logger.Enabled(ctx, slog.LevelError+100))
	})

	t.Run("logging operations do not panic", func(t *testing.T) {
		logger := DisabledLogger()
		// All these should work without panicking
		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")
		logger.Log(context.Background(), slog.LevelError, "custom level")
	})

	t.Run("is different from default logger", func(t *testing.T) {
		disabled := DisabledLogger()
		defaultLog := DefaultLogger()
		be.True(t, disabled != defaultLog)
	})
}

func TestDisabledHandler(t *testing.T) {
	handler := &disabledHandler{}

	t.Run("Enabled returns false for all levels", func(t *testing.T) {
		ctx := context.Background()
		be.False(t, handler.Enabled(ctx, slog.LevelDebug))
		be.False(t, handler.Enabled(ctx, slog.LevelInfo))
		be.False(t, handler.Enabled(ctx, slog.LevelWarn))
		be.False(t, handler.Enabled(ctx, slog.LevelError))
		be.False(t, handler.Enabled(ctx, slog.Level(-1000)))
		be.False(t, handler.Enabled(ctx, slog.Level(1000)))
	})

	t.Run("Handle returns nil error", func(t *testing.T) {
		ctx := context.Background()
		record := slog.Record{}
		record.Message = "test"
		err := handler.Handle(ctx, record)
		be.NilErr(t, err)
	})

	t.Run("WithAttrs returns same handler", func(t *testing.T) {
		attrs := []slog.Attr{
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		}
		result := handler.WithAttrs(attrs)
		// Verify it returns the same handler (pointer equality)
		be.True(t, result == handler)
	})

	t.Run("WithGroup returns same handler", func(t *testing.T) {
		result := handler.WithGroup("testgroup")
		// Verify it returns the same handler (pointer equality)
		be.True(t, result == handler)
	})

	t.Run("chained operations work", func(t *testing.T) {
		result := handler.
			WithGroup("group1").
			WithAttrs([]slog.Attr{slog.String("key", "val")}).
			WithGroup("group2")
		// Verify it returns the same handler (pointer equality)
		be.True(t, result == handler)

		// Still disabled
		be.False(t, result.Enabled(context.Background(), slog.LevelInfo))
	})
}

func TestLoggingIntegration(t *testing.T) {
	// Save and restore original level
	originalLevel := defaultLevel.Level()
	defer defaultLevel.Set(originalLevel)

	t.Run("default logger respects level changes", func(t *testing.T) {
		logger := DefaultLogger()

		SetDefaultLevel(slog.LevelError)
		be.False(t, logger.Enabled(context.Background(), slog.LevelInfo))

		SetDefaultLevel(slog.LevelDebug)
		be.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("disabled logger ignores level changes", func(t *testing.T) {
		logger := DisabledLogger()

		SetDefaultLevel(slog.LevelDebug)
		be.False(t, logger.Enabled(context.Background(), slog.LevelError))

		SetDefaultLevel(slog.LevelError)
		be.False(t, logger.Enabled(context.Background(), slog.LevelError))
	})
}

func TestLoggerTypes(t *testing.T) {
	t.Run("all loggers implement *slog.Logger", func(t *testing.T) {
		var _ *slog.Logger = DefaultLogger()
		var _ *slog.Logger = DisabledLogger()
	})

	t.Run("handler implements slog.Handler", func(t *testing.T) {
		var _ slog.Handler = &disabledHandler{}
	})
}
