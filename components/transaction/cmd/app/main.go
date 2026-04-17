// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
)

// @title						Midaz Transaction API
// @version					v1.48.0
// @description				This is a swagger documentation for the Midaz Transaction API
// @termsOfService				http://swagger.io/terms/
// @contact.name				Discord community
// @contact.url				https://discord.gg/DnhqKwkGv3
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
// @host						localhost:3001
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.

// runner is the minimal contract the transaction binary needs from the
// bootstrap Service. It exists so tests can inject a fake without spinning
// up the real infrastructure (Postgres/Mongo/Redis/Redpanda/gRPC).
type runner interface {
	Run()
}

// deps injects the side-effectful constructors. Real impls in realDeps() call
// into libCommons/libZap/bootstrap; tests substitute fakes.
type deps struct {
	initEnvConfig func() error
	initLogger    func() (libLog.Logger, error)
	initService   func(logger libLog.Logger) (runner, error)
}

// realDeps wires the production constructors. The env-config call returns nil
// because libCommons.InitLocalEnvConfig is void; we preserve its current
// fire-and-forget behavior rather than inventing errors it never had.
func realDeps() deps {
	return deps{
		initEnvConfig: func() error {
			libCommons.InitLocalEnvConfig()
			return nil
		},
		initLogger: libZap.InitializeLoggerWithError,
		initService: func(logger libLog.Logger) (runner, error) {
			svc, err := bootstrap.InitServersWithOptions(&bootstrap.Options{
				Logger: logger,
			})
			if err != nil {
				return nil, fmt.Errorf("init transaction service: %w", err)
			}

			return svc, nil
		},
	}
}

// run executes the transaction bootstrap sequence and returns a process exit
// code. main() is a one-liner around this so the bootstrap is trivially
// unit-testable. Behavior must remain identical to the pre-refactor main():
//   - --healthcheck exits 0 without side effects
//   - env/logger/service init order is preserved
//   - logger.Sync is called on service-init failure only (matches original flow)
//   - service.Run() has no error return; failure modes stay inside the service
func run(args []string, stderr io.Writer, d deps) int {
	if len(args) > 1 && args[1] == "--healthcheck" {
		return 0
	}

	if err := d.initEnvConfig(); err != nil {
		fmt.Fprintf(stderr, "failed to initialize env config: %v\n", err)
		return 1
	}

	logger, err := d.initLogger()
	if err != nil {
		fmt.Fprintf(stderr, "failed to initialize logger: %v\n", err)
		return 1
	}

	service, err := d.initService(logger)
	if err != nil {
		logger.Errorf("Failed to initialize transaction service: %v", err)
		_ = logger.Sync()

		return 1
	}

	service.Run()

	return 0
}

func main() {
	os.Exit(run(os.Args, os.Stderr, realDeps()))
}
