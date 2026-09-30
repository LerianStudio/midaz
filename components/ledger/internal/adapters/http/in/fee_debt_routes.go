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

// RegisterFeeDebtRoutes registers the two read-only fee-debt operations on the given
// Huma API. Paths are GROUP-RELATIVE; the guard chain is attached by
// RegisterFeeDebtV2RoutesToApp.
func RegisterFeeDebtRoutes(api huma.API, h *FeeDebtHandler, opSuffix string) {
	const (
		listPath = "/organizations/{organization_id}/ledgers/{ledger_id}/fee-debts"
		tag      = "Fee Debts"
	)

	huma.Register(api, huma.Operation{
		OperationID: "listFeeDebts" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath,
		Summary:     "List the fee debts of a ledger, oldest first",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.ListFeeDebtsV2)

	huma.Register(api, huma.Operation{
		OperationID: "getFeeDebt" + opSuffix,
		Method:      http.MethodGet,
		Path:        listPath + "/{debt_id}",
		Summary:     "Get a fee debt",
		Tags:        []string{tag},
		Security:    secBillingBearer,
	}, h.GetFeeDebtV2)
}

// RegisterFeeDebtV2RoutesToApp serves the fee-debt surface on /v2 only, under
// auth.Authorize("midaz","fee-debts","get") and the fees-scoped tenant options.
// debt_id is not a UUID, so ParseUUIDPathParameters validates only the scope ids.
func RegisterFeeDebtV2RoutesToApp(group fiber.Router, api huma.API, auth *middleware.AuthClient, h *FeeDebtHandler, routeOptions *pkgHTTP.ProtectedRouteOptions) {
	const feeDebtsPath = "/organizations/:organization_id/ledgers/:ledger_id/fee-debts"

	parse := pkgHTTP.ParseUUIDPathParameters("fee-debts")

	routeGet(group, feeDebtsPath, protectedMidaz(auth, feeDebtsPath, "fee-debts", "get", routeOptions, parse))
	routeGet(group, feeDebtsPath+"/:debt_id", protectedMidaz(auth, feeDebtsPath+"/:debt_id", "fee-debts", "get", routeOptions, parse))

	RegisterFeeDebtRoutes(api, h, v2OpSuffix)
}
