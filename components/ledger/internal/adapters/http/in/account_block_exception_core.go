// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// AccountBlockExceptionHandler serves the account block-exception surface. It is
// a resource of its own rather than an operation on AccountHandler precisely so
// its authz tuple is its own: minting a bypass of the account block is a
// privileged action, distinct from creating or updating an account.
//
// It carries no Query, unlike its sibling handlers: the surface is write-only.
// An exception is presented in a transaction body, never listed or fetched, so
// there is no read to serve.
type AccountBlockExceptionHandler struct {
	Command *command.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// createAccountBlockExceptions owns the span and the service call for an
// already-decoded payload; body decode+validation happens BEFORE this core, in
// the handler, via http.DecodeAndValidate(RawBody). It takes primitive args, so
// nothing transport-shaped reaches it.

// createAccountBlockExceptions delegates the batch to the command.
//
// No span attribute carries an alias or an amount: the amounts are financial
// values and the batch is a security-sensitive grant, so only the command's own
// counts are recorded (see command.CreateAccountBlockExceptions).
func (handler *AccountBlockExceptionHandler) createAccountBlockExceptions(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountBlockExceptionsInput) (*mmodel.AccountBlockExceptions, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_block_exceptions")
	defer span.End()

	exceptions, err := handler.Command.CreateAccountBlockExceptions(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create account block exceptions", err)

		return nil, err
	}

	return exceptions, nil
}
