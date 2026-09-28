// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// FeeApplier drives the in-process fee engine over a transaction's validated
// send/distribute structure. It is the narrow port the transaction create path
// depends on so the fee use case can be injected at bootstrap and faked in
// tests. The signature mirrors fees services.UseCase.CalculateFee: the engine
// mutates cf.Transaction.Send.* in place (legs are appended to Source.From /
// Distribute.To, and Send.Value moves on deductible fees) and returns a
// business error when a package rule rejects the transaction.
type FeeApplier interface {
	CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) error
}
