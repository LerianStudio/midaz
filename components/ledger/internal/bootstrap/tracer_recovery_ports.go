// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=tracer_recovery_ports.go -destination=mock_tracer_recovery_ports_test.go -package=bootstrap

type tracerRecoveryProcessor interface {
	RunOnce(context.Context) (command.TracerRecoverySummary, error)
}

// Discovery must survive an empty or expired local tenant cache after restart.
type tracerRecoveryCatalog interface {
	GetActiveTenantsByService(context.Context, string) ([]*tmclient.TenantSummary, error)
}

type tracerRecoveryPoolResolver interface {
	GetDB(context.Context, string) (dbresolver.DB, error)
}
