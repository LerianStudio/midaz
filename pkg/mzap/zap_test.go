package mzap

import (
	"testing"

	"go.uber.org/zap"
)

func TestZap(t *testing.T) {
	t.Run("log with hydration", func(t *testing.T) {
		l := &ZapWithTraceLogger{}
		l.logWithHydration(func(a ...any) {}, "")
	})

	t.Run("logf with hydration", func(t *testing.T) {
		l := &ZapWithTraceLogger{}
		l.logfWithHydration(func(s string, a ...any) {}, "", "")
	})

	t.Run("ZapWithTraceLogger info", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Info(func(s string, a ...any) {}, "", "")
	})

	t.Run("ZapWithTraceLogger infof", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Infof("", "")
	})

	t.Run("ZapWithTraceLogger infoln", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Infoln("", "")
	})

	t.Run("ZapWithTraceLogger Error", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Error("", "")
	})

	t.Run("ZapWithTraceLogger Errorf", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Errorf("", "")
	})

	t.Run("ZapWithTraceLogger Errorln", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Errorln("", "")
	})

	t.Run("ZapWithTraceLogger Warn", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Warn("", "")
	})

	t.Run("ZapWithTraceLogger Warnf", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Warnf("", "")
	})

	t.Run("ZapWithTraceLogger Warnln", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Warnln("", "")
	})

	t.Run("ZapWithTraceLogger Debug", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Debug("", "")
	})

	t.Run("ZapWithTraceLogger Debugf", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Debugf("", "")
	})

	t.Run("ZapWithTraceLogger Debugln", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Debugln("", "")
	})

	t.Run("ZapWithTraceLogger WithFields", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.WithFields("", "")
	})

	t.Run("ZapWithTraceLogger Sync)", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.Sync()
	})

	t.Run("ZapWithTraceLogger WithDefaultMessageTemplate)", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		sugar := logger.Sugar()

		zapLogger := &ZapWithTraceLogger{
			Logger:                 sugar,
			defaultMessageTemplate: "default template: ",
		}
		zapLogger.WithDefaultMessageTemplate("")
	})

	t.Run("ZapWithTraceLogger WithDefaultMessageTemplate)", func(t *testing.T) {
		hydrateArgs("", []any{})
	})
}
