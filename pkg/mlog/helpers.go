package mlog

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// EnrichTransaction is a convenience function to enrich wide event with transaction context.
// Call this at the start of transaction handlers after extracting IDs.
func EnrichTransaction(c *fiber.Ctx, orgID, ledgerID uuid.UUID, txnType string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	event.SetTransaction("", txnType, "", "")
}

// EnrichTransactionResult updates the wide event with the created/updated transaction.
func EnrichTransactionResult(c *fiber.Ctx, txnID uuid.UUID, status string, opCount int) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetTransactionResult(txnID, opCount, status)
}

// EnrichAccount is a convenience function to enrich wide event with account context.
func EnrichAccount(c *fiber.Ctx, orgID, ledgerID, accountID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if accountID != uuid.Nil {
		event.SetAccount(accountID.String())
	}
}

// EnrichBalance is a convenience function to enrich wide event with balance context.
func EnrichBalance(c *fiber.Ctx, orgID, ledgerID, accountID, balanceID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if accountID != uuid.Nil {
		event.SetAccount(accountID.String())
	}

	if balanceID != uuid.Nil {
		event.SetBalance(balanceID.String())
	}
}

// EnrichOperation is a convenience function to enrich wide event with operation context.
func EnrichOperation(c *fiber.Ctx, orgID, ledgerID, txnID, opID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if txnID != uuid.Nil {
		event.SetTransactionID(txnID.String())
	}

	if opID != uuid.Nil {
		event.SetOperation(opID.String(), 0)
	}
}

// EnrichError adds error context to the wide event.
// Call this before returning an error response.
func EnrichError(c *fiber.Ctx, err error, retryable bool) {
	event := GetWideEvent(c)
	if event == nil || err == nil {
		return
	}

	errType := fmt.Sprintf("%T", err)
	event.SetError(errType, "", err.Error(), retryable)
}

// EnrichErrorWithCode adds error context with a specific code.
func EnrichErrorWithCode(c *fiber.Ctx, err error, code string, retryable bool) {
	event := GetWideEvent(c)
	if event == nil || err == nil {
		return
	}

	errType := fmt.Sprintf("%T", err)
	event.SetError(errType, code, err.Error(), retryable)
}

// SetHandler sets a custom field indicating which handler processed the request.
func SetHandler(c *fiber.Ctx, handlerName string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetCustom("handler", handlerName)
}

// TrackDBQuery increments the DB query counter.
// Call this after each database operation for performance tracking.
func TrackDBQuery(c *fiber.Ctx, durationMS int64) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.IncrementDBMetrics(1, float64(durationMS))
}

// TrackCacheAccess records cache hit/miss.
func TrackCacheAccess(c *fiber.Ctx, hit bool) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if hit {
		event.IncrementCacheHit()
	} else {
		event.IncrementCacheMiss()
	}
}

// TrackExternalCall tracks external service calls.
func TrackExternalCall(c *fiber.Ctx, durationMS int64) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.IncrementExternalCallMetrics(1, float64(durationMS))
}

// SetIdempotency sets idempotency context on the wide event.
// The key is automatically hashed for security.
func SetIdempotency(c *fiber.Ctx, key string, hit bool) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetIdempotency(key, hit)
}

// EnrichTransactionAction sets context for transaction state changes.
// Use for commit, cancel, revert operations on existing transactions.
func EnrichTransactionAction(c *fiber.Ctx, orgID, ledgerID, txnID uuid.UUID, action string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if txnID != uuid.Nil {
		event.SetTransactionID(txnID.String())
	}

	event.SetCustom("transaction_action", action)
}

// EnrichHolder sets holder context on the wide event.
func EnrichHolder(c *fiber.Ctx, holderID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if holderID != uuid.Nil {
		event.SetHolder(holderID.String())
	}
}

// EnrichPortfolio sets portfolio context on the wide event.
func EnrichPortfolio(c *fiber.Ctx, portfolioID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if portfolioID != uuid.Nil {
		event.SetPortfolio(portfolioID.String())
	}
}

// EnrichAssetRate sets asset rate context on the wide event.
func EnrichAssetRate(c *fiber.Ctx, externalID string) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	event.SetAssetRateExternalID(externalID)
}

// EnrichOperationRoute sets operation route context on the wide event.
func EnrichOperationRoute(c *fiber.Ctx, orgID, ledgerID, routeID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if routeID != uuid.Nil {
		event.SetOperationRoute(routeID.String())
	}
}

// EnrichTransactionRoute sets transaction route context on the wide event.
func EnrichTransactionRoute(c *fiber.Ctx, orgID, ledgerID, routeID uuid.UUID) {
	event := GetWideEvent(c)
	if event == nil {
		return
	}

	if orgID != uuid.Nil {
		event.SetOrganization(orgID.String())
	}

	if ledgerID != uuid.Nil {
		event.SetLedger(ledgerID.String())
	}

	if routeID != uuid.Nil {
		event.SetTransactionRoute(routeID.String())
	}
}
