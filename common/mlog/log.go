package mlog

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// Logger is the common interface for log implementation.
type Logger interface {
	Info(args ...any)
	Infof(format string, args ...any)
	Infoln(args ...any)

	Error(args ...any)
	Errorf(format string, args ...any)
	Errorln(args ...any)

	Warn(args ...any)
	Warnf(format string, args ...any)
	Warnln(args ...any)

	Debug(args ...any)
	Debugf(format string, args ...any)
	Debugln(args ...any)

	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Fatalln(args ...any)

	WithFields(fields ...any) Logger

	Sync() error
}

// LogLevel represents the level of log system (fatal, error, warn, info and debug).
type LogLevel int8

// These are the different log levels. You can set the logging level to log.
const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel LogLevel = iota
	// FatalLevel level. Logs and then calls `logger.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
)

// ParseLevel takes a string level and returns a LogLevel constant.
func ParseLevel(lvl string) (LogLevel, error) {
	switch strings.ToLower(lvl) {
	case "fatal":
		return FatalLevel, nil
	case "error":
		return ErrorLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "info":
		return InfoLevel, nil
	case "debug":
		return DebugLevel, nil
	}

	var l LogLevel

	return l, fmt.Errorf("not a valid LogLevel: %q", lvl)
}

// GoLogger is the Go built-in (log) implementation of Logger interface.
type GoLogger struct {
	fields []any
	Level  LogLevel
}

// IsLevelEnabled checks if the given level is enabled.
func (l *GoLogger) IsLevelEnabled(level LogLevel) bool {
	return l.Level >= level
}

// Info implements Info Logger interface function.
func (l *GoLogger) Info(args ...any) {
	if l.IsLevelEnabled(InfoLevel) {
		log.Print(args...)
	}
}

// Infof implements Infof Logger interface function.
func (l *GoLogger) Infof(format string, args ...any) {
	if l.IsLevelEnabled(InfoLevel) {
		log.Printf(format, args...)
	}
}

// Infoln implements Infoln Logger interface function.
func (l *GoLogger) Infoln(args ...any) {
	if l.IsLevelEnabled(InfoLevel) {
		log.Println(args...)
	}
}

// Error implements Error Logger interface function.
func (l *GoLogger) Error(args ...any) {
	if l.IsLevelEnabled(ErrorLevel) {
		log.Print(args...)
	}
}

// Errorf implements Errorf Logger interface function.
func (l *GoLogger) Errorf(format string, args ...any) {
	if l.IsLevelEnabled(ErrorLevel) {
		log.Printf(format, args...)
	}
}

// Errorln implements Errorln Logger interface function.
func (l *GoLogger) Errorln(args ...any) {
	if l.IsLevelEnabled(ErrorLevel) {
		log.Println(args...)
	}
}

// Warn implements Warn Logger interface function.
func (l *GoLogger) Warn(args ...any) {
	if l.IsLevelEnabled(WarnLevel) {
		log.Print(args...)
	}
}

// Warnf implements Warnf Logger interface function.
func (l *GoLogger) Warnf(format string, args ...any) {
	if l.IsLevelEnabled(WarnLevel) {
		log.Printf(format, args...)
	}
}

// Warnln implements Warnln Logger interface function.
func (l *GoLogger) Warnln(args ...any) {
	if l.IsLevelEnabled(WarnLevel) {
		log.Println(args...)
	}
}

// Debug implements Debug Logger interface function.
func (l *GoLogger) Debug(args ...any) {
	if l.IsLevelEnabled(DebugLevel) {
		log.Print(args...)
	}
}

// Debugf implements Debugf Logger interface function.
func (l *GoLogger) Debugf(format string, args ...any) {
	if l.IsLevelEnabled(DebugLevel) {
		log.Printf(format, args...)
	}
}

// Debugln implements Debugln Logger interface function.
func (l *GoLogger) Debugln(args ...any) {
	if l.IsLevelEnabled(DebugLevel) {
		log.Println(args...)
	}
}

// Fatal implements Fatal Logger interface function.
func (l *GoLogger) Fatal(args ...any) {
	if l.IsLevelEnabled(FatalLevel) {
		log.Print(args...)
	}
}

// Fatalf implements Fatalf Logger interface function.
func (l *GoLogger) Fatalf(format string, args ...any) {
	if l.IsLevelEnabled(FatalLevel) {
		log.Printf(format, args...)
	}
}

// Fatalln implements Fatalln Logger interface function.
func (l *GoLogger) Fatalln(args ...any) {
	if l.IsLevelEnabled(FatalLevel) {
		log.Println(args...)
	}
}

// WithFields implements WithFields Logger interface function
//
//nolint:ireturn
func (l *GoLogger) WithFields(fields ...any) Logger {
	return &GoLogger{
		Level:  l.Level,
		fields: fields,
	}
}

// Sync implements Sync Logger interface function.
//
//nolint:ireturn
func (l *GoLogger) Sync() error { return nil }

// NewLoggerFromContext extract the Logger from "logger" value inside context
//
//nolint:ireturn
func NewLoggerFromContext(ctx context.Context) Logger {
	if logger := ctx.Value(loggerContextKey("logger")); logger != nil {
		if l, ok := logger.(Logger); ok {
			return l
		}
	}

	return &NoneLogger{}
}

type loggerContextKey string

// ContextWithLogger returns a context within a Logger in "logger" value.
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey("logger"), logger)
}
