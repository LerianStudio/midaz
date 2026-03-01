// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package main is the entry point for the standalone consumer service.
// This service reads messages from Redpanda and persists them to PostgreSQL and MongoDB.
// It is the dedicated persistence worker extracted from the ledger binary to achieve
// clean architectural separation between the API path (ledger) and the write path (consumer).
package main

import (
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		return
	}

	libCommons.InitLocalEnvConfig()

	logger, err := libZap.InitializeLoggerWithError()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)

		os.Exit(1)
	}

	service, err := transaction.InitConsumerServiceOrError(&transaction.Options{
		Logger: logger,
	})
	if err != nil {
		logger.Errorf("Failed to initialize consumer service: %v", err)
		_ = logger.Sync()

		os.Exit(1)
	}

	service.Run()
}
