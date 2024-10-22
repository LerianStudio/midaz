package mzap

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
)

// ZapWithTraceLogger is a wrapper of otelzap.SugaredLogger.
type ZapWithTraceLogger struct {
	Logger *otelzap.SugaredLogger
}

// Info implements Info Logger interface function.
func (l *ZapWithTraceLogger) Info(args ...any) { l.Logger.Info(args...) }

// Infof implements Infof Logger interface function.
func (l *ZapWithTraceLogger) Infof(format string, args ...any) { l.Logger.Infof(format, args...) }

// Infoln implements Infoln Logger interface function.
func (l *ZapWithTraceLogger) Infoln(args ...any) { l.Logger.Infoln(args...) }

// InfofContext implements InfofContext function from otelzap which uses context.Context to log with span information if available.
func (l *ZapWithTraceLogger) InfofContext(ctx context.Context, format string, args ...any) {
	l.Logger.InfofContext(ctx, format, args...)
}

// InfowContext implements InfowContext function from otelzap which uses context.Context to log with span information if available and the key-value pairs as structured context.
func (l *ZapWithTraceLogger) InfowContext(ctx context.Context, format string, keysAndValues ...any) {
	l.Logger.InfowContext(ctx, format, keysAndValues)
}

// Error implements Error Logger interface function.
func (l *ZapWithTraceLogger) Error(args ...any) { l.Logger.Error(args...) }

// Errorf implements Errorf Logger interface function.
func (l *ZapWithTraceLogger) Errorf(format string, args ...any) { l.Logger.Errorf(format, args...) }

// Errorln implements Errorln Logger interface function
func (l *ZapWithTraceLogger) Errorln(args ...any) { l.Logger.Errorln(args...) }

// ErrorfContext implements ErrorfContext function from otelzap which uses context.Context to log with span information if available.
func (l *ZapWithTraceLogger) ErrorfContext(ctx context.Context, format string, args ...any) {
	l.Logger.ErrorfContext(ctx, format, args...)
}

// ErrorwContext implements ErrorwContext function from otelzap which uses context.Context to log with span information if available and the key-value pairs as structured context.
func (l *ZapWithTraceLogger) ErrorwContext(ctx context.Context, format string, keysAndValues ...any) {
	l.Logger.ErrorwContext(ctx, format, keysAndValues)
}

// Warn implements Warn Logger interface function.
func (l *ZapWithTraceLogger) Warn(args ...any) { l.Logger.Warn(args...) }

// Warnf implements Warnf Logger interface function.
func (l *ZapWithTraceLogger) Warnf(format string, args ...any) { l.Logger.Warnf(format, args...) }

// Warnln implements Warnln Logger interface function
func (l *ZapWithTraceLogger) Warnln(args ...any) { l.Logger.Warnln(args...) }

// WarnfContext implements WarnfContext function from otelzap which uses context.Context to log with span information if available.
func (l *ZapWithTraceLogger) WarnfContext(ctx context.Context, format string, args ...any) {
	l.Logger.WarnfContext(ctx, format, args...)
}

// WarnwContext implements WarnwContext function from otelzap which uses context.Context to log with span information if available and the key-value pairs as structured context.
func (l *ZapWithTraceLogger) WarnwContext(ctx context.Context, format string, keysAndValues ...any) {
	l.Logger.WarnwContext(ctx, format, keysAndValues)
}

// Debug implements Debug Logger interface function.
func (l *ZapWithTraceLogger) Debug(args ...any) { l.Logger.Debug(args...) }

// Debugf implements Debugf Logger interface function.
func (l *ZapWithTraceLogger) Debugf(format string, args ...any) { l.Logger.Debugf(format, args...) }

// Debugln implements Debugln Logger interface function
func (l *ZapWithTraceLogger) Debugln(args ...any) { l.Logger.Debugln(args...) }

// DebugfContext implements DebugfContext function from otelzap which uses context.Context to log with span information if available.
func (l *ZapWithTraceLogger) DebugfContext(ctx context.Context, format string, args ...any) {
	l.Logger.DebugfContext(ctx, format, args...)
}

// DebugwContext implements DebugwContext function from otelzap which uses context.Context to log with span information if available and the key-value pairs as structured context.
func (l *ZapWithTraceLogger) DebugwContext(ctx context.Context, format string, keysAndValues ...any) {
	l.Logger.DebugwContext(ctx, format, keysAndValues)
}

// Fatal implements Fatal Logger interface function.
func (l *ZapWithTraceLogger) Fatal(args ...any) { l.Logger.Fatal(args...) }

// Fatalf implements Fatalf Logger interface function.
func (l *ZapWithTraceLogger) Fatalf(format string, args ...any) { l.Logger.Fatalf(format, args...) }

// Fatalln implements Fatalln Logger interface function
func (l *ZapWithTraceLogger) Fatalln(args ...any) { l.Logger.Fatalln(args...) }

// FatalfContext implements FatalfContext function from otelzap which uses context.Context to log with span information if available.
func (l *ZapWithTraceLogger) FatalfContext(ctx context.Context, format string, args ...any) {
	l.Logger.FatalfContext(ctx, format, args...)
}

// FatalwContext implements FatalwContext function from otelzap which uses context.Context to log with span information if available and the key-value pairs as structured context.
func (l *ZapWithTraceLogger) FatalwContext(ctx context.Context, format string, keysAndValues ...any) {
	l.Logger.FatalwContext(ctx, format, keysAndValues)
}

// WithFields adds structured context to the logger. It returns a new logger and leaves the original unchanged.
//
//nolint:ireturn
func (l *ZapWithTraceLogger) WithFields(fields ...any) mlog.Logger {
	newLogger := l.Logger.With(fields...)

	return &ZapWithTraceLogger{
		Logger: newLogger,
	}
}
