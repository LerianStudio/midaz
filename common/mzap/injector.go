package mzap

import (
	"context"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/log/global"
	"log"
	"os"

	"github.com/LerianStudio/midaz/common/mlog"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitializeLogger initializes our log layer and returns it
//
//nolint:ireturn
func InitializeLogger() mlog.Logger {
	var zapCfg zap.Config

	ctx := context.Background()

	if os.Getenv("ENV_NAME") == "production" {
		zapCfg = zap.NewProductionConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if val, ok := os.LookupEnv("LOG_LEVEL"); ok {
		var lvl zapcore.Level
		if err := lvl.Set(val); err != nil {
			log.Printf("Invalid LOG_LEVEL, fallback to InfoLevel: %v", err)

			lvl = zapcore.InfoLevel
		}

		zapCfg.Level = zap.NewAtomicLevelAt(lvl)
	}

	zapCfg.DisableStacktrace = true

	t := mopentelemetry.Telemetry{}

	lExp, err := t.NewLoggerExporter(ctx)
	if err != nil {
		log.Fatalf("can't initialize logger exporter: %v", err)
	}

	r, err := t.NewResource()
	if err != nil {
		log.Fatalf("can't initialize logger resource: %v", err)
	}

	lp := t.NewLoggerProvider(r, lExp)
	global.SetLoggerProvider(lp)

	logger, err := zapCfg.Build(zap.AddCallerSkip(1), zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, otelzap.NewCore(os.Getenv("OTEL_LIBRARY_NAME")))
	}))
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}

	sugarLogger := logger.Sugar()

	sugarLogger.Infof("Log level is (%v)", zapCfg.Level)
	sugarLogger.Infof("Logger is (%T) \n", sugarLogger)

	return &ZapWithTraceLogger{
		Logger:         sugarLogger,
		LoggerProvider: lp,
		shutdown: func() {
			err := lExp.Shutdown(ctx)
			if err != nil {
				sugarLogger.Fatalf("can't shutdown logger exporter: %v", err)
			}

			err = lp.Shutdown(ctx)
			if err != nil {
				sugarLogger.Fatalf("can't shutdown logger provider: %v", err)
			}
		},
	}
}
