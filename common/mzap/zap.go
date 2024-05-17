package mzap

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"go.uber.org/zap"
)

// ZapLogger is a wrapper of zap.SugaredLogger.
type ZapLogger struct {
	Logger *zap.SugaredLogger
}

// Info implements Info Logger interface function.
func (l *ZapLogger) Info(args ...any) { l.Logger.Info(args...) }

// Infof implements Infof Logger interface function.
func (l *ZapLogger) Infof(format string, args ...any) { l.Logger.Infof(format, args...) }

// Infoln implements Infoln Logger interface function.
func (l *ZapLogger) Infoln(args ...any) { l.Logger.Infoln(args...) }

// Error implements Error Logger interface function.
func (l *ZapLogger) Error(args ...any) { l.Logger.Error(args...) }

// Errorf implements Errorf Logger interface function.
func (l *ZapLogger) Errorf(format string, args ...any) { l.Logger.Errorf(format, args...) }

// Errorln implements Errorln Logger interface function
func (l *ZapLogger) Errorln(args ...any) { l.Logger.Errorln(args...) }

// Warn implements Warn Logger interface function.
func (l *ZapLogger) Warn(args ...any) { l.Logger.Warn(args...) }

// Warnf implements Warnf Logger interface function.
func (l *ZapLogger) Warnf(format string, args ...any) { l.Logger.Warnf(format, args...) }

// Warnln implements Warnln Logger interface function
func (l *ZapLogger) Warnln(args ...any) { l.Logger.Warnln(args...) }

// Debug implements Debug Logger interface function.
func (l *ZapLogger) Debug(args ...any) { l.Logger.Debug(args...) }

// Debugf implements Debugf Logger interface function.
func (l *ZapLogger) Debugf(format string, args ...any) { l.Logger.Debugf(format, args...) }

// Debugln implements Debugln Logger interface function
func (l *ZapLogger) Debugln(args ...any) { l.Logger.Debugln(args...) }

// Fatal implements Fatal Logger interface function.
func (l *ZapLogger) Fatal(args ...any) { l.Logger.Fatal(args...) }

// Fatalf implements Fatalf Logger interface function.
func (l *ZapLogger) Fatalf(format string, args ...any) { l.Logger.Fatalf(format, args...) }

// Fatalln implements Fatalln Logger interface function
func (l *ZapLogger) Fatalln(args ...any) { l.Logger.Fatalln(args...) }

// WithFields adds structured context to the logger. It returns a new logger and leaves the original unchanged.
//
//nolint:ireturn
func (l *ZapLogger) WithFields(fields ...any) mlog.Logger {
	newLogger := l.Logger.With(fields...)

	return &ZapLogger{
		Logger: newLogger,
	}
}
