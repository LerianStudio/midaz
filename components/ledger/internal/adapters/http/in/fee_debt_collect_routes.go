// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RegisterFeeDebtCollectV2RoutesToApp serves the standalone collection on /v2 only,
// under auth.Authorize("midaz","fee-debts","post") and the transaction-scoped tenant
// options, because it moves money in the ledger's transaction stores.
func RegisterFeeDebtCollectV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *TransactionHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	routePost(group, "/organizations/:organization_id/ledgers/:ledger_id/fee-debts/collect",
		protectedMidaz(auth, "fee-debts", "post", routeOptions, pkgHTTP.ParseUUIDPathParameters("fee-debts")))

	huma.Register(api, huma.Operation{
		OperationID: "collectFeeDebts" + v2OpSuffix,
		Method:      http.MethodPost,
		Path:        "/organizations/{organization_id}/ledgers/{ledger_id}/fee-debts/collect",
		Summary:     "Collect a balance's open fee debts",
		Description: "Settles the balance's open fee debts oldest first from its available funds, up to maxAmount and never more than it owes, in one transaction. " +
			"It charges no new fee. Without an error, it collects nothing when the balance has no available funds or is blocked, cannot send, is closing or closed, deleted, debit-direction or external, " +
			"and it stops at the first debt whose fee account is blocked, cannot receive, is closing or closed, deleted, debit-direction, external, in overdraft or of another asset. " +
			"When nothing is settled the response is collected 0 and no transaction is created. With X-Idempotency a retry returns the first answer; without it every call is a new collection. " +
			"A collection transaction cannot be reverted.",
		Tags:             []string{"Fee Debts"},
		Security:         secBillingBearer,
		SkipValidateBody: true,
	}, h.CollectFeeDebtV2)
	attachTypedRequestBody[FeeDebtCollectInput](api, "collectFeeDebts"+v2OpSuffix)
}
