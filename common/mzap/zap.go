package mzap

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"go.uber.org/zap"
)

// ZapWithTraceLogger is a wrapper of otelzap.SugaredLogger.
//
// It implements Logger interface.
// The shutdown function is used to close the logger provider.
type ZapWithTraceLogger struct {
	Logger *zap.SugaredLogger
}

// Info implements Info Logger interface function.
func (l *ZapWithTraceLogger) Info(args ...any) { l.Logger.Info(args...) }

// Infof implements Infof Logger interface function.
func (l *ZapWithTraceLogger) Infof(format string, args ...any) { l.Logger.Infof(format, args...) }

// Infoln implements Infoln Logger interface function.
func (l *ZapWithTraceLogger) Infoln(args ...any) { l.Logger.Infoln(args...) }

// Error implements Error Logger interface function.
func (l *ZapWithTraceLogger) Error(args ...any) { l.Logger.Error(args...) }

// Errorf implements Errorf Logger interface function.
func (l *ZapWithTraceLogger) Errorf(format string, args ...any) { l.Logger.Errorf(format, args...) }

// Errorln implements Errorln Logger interface function
func (l *ZapWithTraceLogger) Errorln(args ...any) { l.Logger.Errorln(args...) }

// Warn implements Warn Logger interface function.
func (l *ZapWithTraceLogger) Warn(args ...any) { l.Logger.Warn(args...) }

// Warnf implements Warnf Logger interface function.
func (l *ZapWithTraceLogger) Warnf(format string, args ...any) { l.Logger.Warnf(format, args...) }

// Warnln implements Warnln Logger interface function
func (l *ZapWithTraceLogger) Warnln(args ...any) { l.Logger.Warnln(args...) }

// Debug implements Debug Logger interface function.
func (l *ZapWithTraceLogger) Debug(args ...any) { l.Logger.Debug(args...) }

// Debugf implements Debugf Logger interface function.
func (l *ZapWithTraceLogger) Debugf(format string, args ...any) { l.Logger.Debugf(format, args...) }

// Debugln implements Debugln Logger interface function
func (l *ZapWithTraceLogger) Debugln(args ...any) { l.Logger.Debugln(args...) }

// Fatal implements Fatal Logger interface function.
func (l *ZapWithTraceLogger) Fatal(args ...any) { l.Logger.Fatal(args...) }

// Fatalf implements Fatalf Logger interface function.
func (l *ZapWithTraceLogger) Fatalf(format string, args ...any) { l.Logger.Fatalf(format, args...) }

// Fatalln implements Fatalln Logger interface function
func (l *ZapWithTraceLogger) Fatalln(args ...any) { l.Logger.Fatalln(args...) }

// WithFields adds structured context to the logger. It returns a new logger and leaves the original unchanged.
//
//nolint:ireturn
func (l *ZapWithTraceLogger) WithFields(fields ...any) mlog.Logger {
	newLogger := l.Logger.With(fields...)

	return &ZapWithTraceLogger{
		Logger: newLogger,
	}
}

// Sync implements Sync Logger interface function.
//
// Sync calls the underlying Core's Sync method, flushing any buffered log entries as well as closing the logger provider used by open telemetry. Applications should take care to call Sync before exiting.
//
//nolint:ireturn
func (l *ZapWithTraceLogger) Sync() error {
	err := l.Logger.Sync()
	if err != nil {
		return err
	}

	return nil
}
