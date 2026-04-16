// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// recordingLogger is a minimal libLog.Logger implementation that captures
// emitted messages by level so tests can assert audit lines were produced.
type recordingLogger struct {
	mu    sync.Mutex
	infos []string
	warns []string
	errs  []string
}

func (l *recordingLogger) appendInfo(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.infos = append(l.infos, msg)
}

func (l *recordingLogger) appendWarn(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.warns = append(l.warns, msg)
}

func (l *recordingLogger) appendError(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.errs = append(l.errs, msg)
}

func (l *recordingLogger) Info(args ...any) { l.appendInfo(fmt.Sprint(args...)) }
func (l *recordingLogger) Infof(format string, args ...any) {
	l.appendInfo(fmt.Sprintf(format, args...))
}
func (l *recordingLogger) Infoln(args ...any) { l.appendInfo(fmt.Sprintln(args...)) }
func (l *recordingLogger) Error(args ...any)  { l.appendError(fmt.Sprint(args...)) }
func (l *recordingLogger) Errorf(format string, args ...any) {
	l.appendError(fmt.Sprintf(format, args...))
}
func (l *recordingLogger) Errorln(args ...any) { l.appendError(fmt.Sprintln(args...)) }
func (l *recordingLogger) Warn(args ...any)    { l.appendWarn(fmt.Sprint(args...)) }
func (l *recordingLogger) Warnf(format string, args ...any) {
	l.appendWarn(fmt.Sprintf(format, args...))
}
func (l *recordingLogger) Warnln(args ...any)        { l.appendWarn(fmt.Sprintln(args...)) }
func (l *recordingLogger) Debug(_ ...any)            {}
func (l *recordingLogger) Debugf(_ string, _ ...any) {}
func (l *recordingLogger) Debugln(_ ...any)          {}
func (l *recordingLogger) Fatal(args ...any)         { l.appendError(fmt.Sprint(args...)) }
func (l *recordingLogger) Fatalf(format string, args ...any) {
	l.appendError(fmt.Sprintf(format, args...))
}
func (l *recordingLogger) Fatalln(args ...any)               { l.appendError(fmt.Sprintln(args...)) }
func (l *recordingLogger) WithFields(_ ...any) libLog.Logger { return l }
func (l *recordingLogger) WithDefaultMessageTemplate(_ string) libLog.Logger {
	return l
}
func (l *recordingLogger) Sync() error { return nil }

func TestBootstrap_RejectsNoopPublisherInProduction(t *testing.T) {
	t.Parallel()

	productionEnvs := []string{
		"production",
		"Production",
		"PROD",
		"prd",
		"staging",
		"stg",
		"pre-prod",
		"preprod",
		"  production  ", // whitespace
	}

	for _, env := range productionEnvs {
		t.Run("env="+env, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{EnvName: env, RedpandaEnabled: false}
			logger := &recordingLogger{}

			err := validatePublisherSelection(cfg, logger)
			require.ErrorIs(t, err, constant.ErrNoopPublisherNotAllowedInProd)

			logger.mu.Lock()
			defer logger.mu.Unlock()

			require.NotEmpty(t, logger.errs, "expected an error-level audit log line")
			assert.Contains(t, logger.errs[0], "publisher audit: selected=noop")
			assert.Contains(t, logger.errs[0], "decision=abort")
		})
	}
}

func TestBootstrap_AllowsNoopPublisherInDevelopment(t *testing.T) {
	t.Parallel()

	devEnvs := []string{"dev", "development", "local", "test", "qa", "sandbox", "ci", ""}

	for _, env := range devEnvs {
		t.Run("env="+env, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{EnvName: env, RedpandaEnabled: false}
			logger := &recordingLogger{}

			err := validatePublisherSelection(cfg, logger)
			require.NoError(t, err)

			logger.mu.Lock()
			defer logger.mu.Unlock()

			require.NotEmpty(t, logger.warns, "expected a warn-level audit log line in dev envs")
			assert.Contains(t, logger.warns[0], "publisher audit: selected=noop")
			assert.Contains(t, logger.warns[0], "SILENT DROP RISK")
		})
	}
}

func TestBootstrap_RedpandaEnabled_EmitsInfoAudit(t *testing.T) {
	t.Parallel()

	cfg := &Config{EnvName: "production", RedpandaEnabled: true}
	logger := &recordingLogger{}

	err := validatePublisherSelection(cfg, logger)
	require.NoError(t, err)

	logger.mu.Lock()
	defer logger.mu.Unlock()

	require.NotEmpty(t, logger.infos, "expected an info-level audit log line")
	assert.Contains(t, logger.infos[0], "publisher audit: selected=redpanda")
}
