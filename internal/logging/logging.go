// Package logging configures the application's structured logger: a
// human-readable stream to the console and a rotating JSON file so a crash
// or misbehaving bind can be diagnosed after the fact.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	Dir        string // directory to write log files into
	FileName   string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Debug      bool // include debug-level logs
}

func DefaultOptions() Options {
	return Options{
		Dir:        "logs",
		FileName:   "streamer-remote.log",
		MaxSizeMB:  10,
		MaxBackups: 5,
		MaxAgeDays: 30,
	}
}

// New builds the process-wide logger and returns it alongside the file
// sink so callers can flush/close it on shutdown.
func New(opts Options) (*slog.Logger, io.Closer) {
	level := slog.LevelInfo
	if opts.Debug {
		level = slog.LevelDebug
	}

	fileSink := &lumberjack.Logger{
		Filename:   opts.Dir + string(os.PathSeparator) + opts.FileName,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   true,
	}

	fileHandler := slog.NewJSONHandler(fileSink, &slog.HandlerOptions{Level: level})
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	logger := slog.New(&multiHandler{handlers: []slog.Handler{fileHandler, consoleHandler}})
	return logger, fileSink
}

// multiHandler fans a log record out to every wrapped handler so a single
// log call reaches both the file and the console.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}
