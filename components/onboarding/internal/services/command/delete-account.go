package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	balanceproto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteAccountByID deletes an account and all associated balances.
//
// This method performs a cascading delete operation that removes both the account
// and all its associated balances. The operation coordinates between the onboarding
// service (account deletion) and the transaction service (balance deletion via gRPC).
//
// Deletion Process:
//
//	Step 1: Context Setup
//	  - Extract logger, tracer, and requestID from context
//	  - Start OpenTelemetry span "command.delete_account_by_id"
//
//	Step 2: Account Validation
//	  - Find account by ID within organization and ledger scope
//	  - If account not found: Return error
//	  - If account is "external" type: Return ErrForbiddenExternalAccountManipulation
//	    (External accounts are system-managed and cannot be deleted)
//
//	Step 3: Balance Deletion (gRPC)
//	  - Build DeleteAllBalancesByAccountIDRequest with account details
//	  - Call BalanceGRPCRepo.DeleteAllBalancesByAccountID via gRPC
//	  - If unauthorized/forbidden: Return auth error as-is
//	  - If other gRPC error: Return ErrAccountBalanceDeletion business error
//
//	Step 4: Account Deletion
//	  - Delete account from PostgreSQL via AccountRepo.Delete
//	  - If account not found during delete: Return ErrAccountIDNotFound
//	  - If other error: Return wrapped error
//
// External Account Protection:
//
// External accounts are automatically created during asset creation to serve as
// the counterparty for external transactions. These accounts:
//   - Have type="external" and status.code="external"
//   - Are named "External {ASSET_CODE}" with alias "@external/{ASSET_CODE}"
//   - Must not be deleted as they are required for asset operations
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the account
//   - portfolioID: Optional portfolio UUID (nil if account has no portfolio)
//   - id: UUID of the account to delete
//   - token: Authentication token for gRPC balance service call
//
// Returns:
//   - error: Business or infrastructure error, nil on success
//
// Error Scenarios:
//   - ErrForbiddenExternalAccountManipulation: Cannot delete external accounts
//   - ErrAccountBalanceDeletion: Failed to delete associated balances
//   - ErrAccountIDNotFound: Account does not exist
//   - UnauthorizedError: Invalid or expired token
//   - ForbiddenError: Insufficient permissions
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, token string) error {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	logger.Infof("Remove account for id: %s", id.String())

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by id", err)

		logger.Errorf("Error finding account by id: %v", err)

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}

	balanceDeleteRequest := &balanceproto.DeleteAllBalancesByAccountIDRequest{
		OrganizationId: organizationID.String(),
		LedgerId:       ledgerID.String(),
		AccountId:      accFound.ID,
		RequestId:      requestID,
	}

	err = uc.BalanceGRPCRepo.DeleteAllBalancesByAccountID(ctx, token, balanceDeleteRequest)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete all balances by account id via gRPC", err)

		logger.Errorf("Failed to delete all balances by account id via gRPC: %v", err)

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Warnf("Account ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account on repo by id", err)

		logger.Errorf("Error deleting account: %v", err)

		return err
	}

	return nil
}
