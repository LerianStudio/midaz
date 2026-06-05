// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Command backfill is a standalone, idempotent maintenance runner that
// provisions deterministic self-holders for existing organizations (Mongo first)
// and materialises account.holder_id for non-external accounts (PostgreSQL).
//
// It is not a SQL migration: it spans two stores in a mandatory order and is safe
// to re-run. In multi-tenant mode it enumerates active tenants and runs one pass
// per tenant; in single-tenant mode it runs one pass against the ambient stores.
package main

import (
	"context"
	"fmt"
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/bootstrap"
)

func main() {
	libCommons.InitLocalEnvConfig()

	runner, err := bootstrap.InitHolderBackfill()
	if err != nil {
		// fmt is used here because the structured logger is initialized inside
		// InitHolderBackfill; this is the only place fmt output is acceptable.
		fmt.Fprintf(os.Stderr, "Failed to initialize holder backfill: %v\n", err)

		os.Exit(1)
	}

	if err := runner.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Holder backfill failed: %v\n", err)

		os.Exit(1)
	}
}
