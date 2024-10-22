package mzap

import (
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
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

	// AddCallerSkip(1) is used to skip the stack frame of the zap logger wrapper code and get the actual caller
	logger, err := zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}

	tracingLogger := otelzap.New(logger)
	sugarLogger := tracingLogger.Sugar()

	sugarLogger.Infof("Log level is (%v)", zapCfg.Level)
	sugarLogger.Infof("Logger is (%T) \n", sugarLogger)

	return &ZapWithTraceLogger{
		Logger: sugarLogger,
	}
}
