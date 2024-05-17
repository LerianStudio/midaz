package mzap

import (
	"fmt"
	"log"
	"os"

	"github.com/LerianStudio/midaz/common/console"
	"github.com/LerianStudio/midaz/common/mlog"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitializeLogger initializes our log layer and returns it
//
//nolint:ireturn
func InitializeLogger() mlog.Logger {
	fmt.Println(console.Title("InitializeLogger"))

	var zapCfg zap.Config

	if os.Getenv("ENV_NAME") == "local" {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	if val, ok := os.LookupEnv("LOG_LEVEL"); ok {
		var lvl zapcore.Level
		if err := lvl.Set(val); err != nil {
			log.Printf("Invalid LOG_LEVEL, fallback to InfoLevel: %v", err)

			lvl = zapcore.InfoLevel
		}

		zapCfg.Level = zap.NewAtomicLevelAt(lvl)
	}

	zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	zapCfg.DisableStacktrace = true

	logger, err := zapCfg.Build()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}

	sugar := logger.Sugar()

	fmt.Printf("Log level is (%v)\n", zapCfg.Level)
	fmt.Printf("Logger is (%T)\n", sugar)

	fmt.Println(console.Line(console.DefaultLineSize))

	return &ZapLogger{
		Logger: sugar,
	}
}
