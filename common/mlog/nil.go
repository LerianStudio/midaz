package mlog

// NoneLogger is a wrapper for log nothing.
type NoneLogger struct{}

// Info implements Info Logger interface function.
func (l *NoneLogger) Info(args ...any) {}

// Infof implements Infof Logger interface function.
func (l *NoneLogger) Infof(format string, args ...any) {}

// Infoln implements Infoln Logger interface function.
func (l *NoneLogger) Infoln(args ...any) {}

// Error implements Error Logger interface function.
func (l *NoneLogger) Error(args ...any) {}

// Errorf implements Errorf Logger interface function.
func (l *NoneLogger) Errorf(format string, args ...any) {}

// Errorln implements Errorln Logger interface function.
func (l *NoneLogger) Errorln(args ...any) {}

// Warn implements Warn Logger interface function.
func (l *NoneLogger) Warn(args ...any) {}

// Warnf implements Warnf Logger interface function.
func (l *NoneLogger) Warnf(format string, args ...any) {}

// Warnln implements Warnln Logger interface function.
func (l *NoneLogger) Warnln(args ...any) {}

// Debug implements Debug Logger interface function.
func (l *NoneLogger) Debug(args ...any) {}

// Debugf implements Debugf Logger interface function.
func (l *NoneLogger) Debugf(format string, args ...any) {}

// Debugln implements Debugln Logger interface function.
func (l *NoneLogger) Debugln(args ...any) {}

// Fatal implements Fatal Logger interface function.
func (l *NoneLogger) Fatal(args ...any) {}

// Fatalf implements Fatalf Logger interface function.
func (l *NoneLogger) Fatalf(format string, args ...any) {}

// Fatalln implements Fatalln Logger interface function.
func (l *NoneLogger) Fatalln(args ...any) {}

// WithFields implements WithFields Logger interface function
//
//nolint:ireturn
func (l *NoneLogger) WithFields(fields ...any) Logger {
	return l
}
