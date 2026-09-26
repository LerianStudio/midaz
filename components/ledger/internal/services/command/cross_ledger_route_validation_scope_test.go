// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// routeValidationScopeCase is a two-ledger group, A to B, where each ledger may
// validate accounting routes and B may belong to another organization.
type routeValidationScopeCase struct {
	name           string
	crossOrg       bool
	validatesA     bool
	validatesB     bool
	wantRefused    bool
	organizationID uuid.UUID
	ledgerA        atomicTransactionBatchLedgerRef
	ledgerB        atomicTransactionBatchLedgerRef
}

func routeValidationScopeCases() []routeValidationScopeCase {
	cases := []routeValidationScopeCase{
		{name: "one organization with validating ledgers", validatesA: true, validatesB: true},
		{name: "one organization with a validating ledger", validatesA: true},
		{name: "two organizations with a validating ledger", crossOrg: true, validatesB: true, wantRefused: true},
		{name: "two organizations with validating ledgers", crossOrg: true, validatesA: true, validatesB: true, wantRefused: true},
		{name: "two organizations without route validation", crossOrg: true},
	}

	for index := range cases {
		organizationID := uuid.MustParse("0199b800-0000-7000-8000-000000000001")
		otherOrganizationID := organizationID
		if cases[index].crossOrg {
			otherOrganizationID = uuid.MustParse("0199b800-0000-7000-8000-000000000002")
		}

		cases[index].organizationID = organizationID
		cases[index].ledgerA = atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199b800-0000-7000-8000-00000000000a")}
		cases[index].ledgerB = atomicTransactionBatchLedgerRef{organizationID: otherOrganizationID, ledgerID: uuid.MustParse("0199b800-0000-7000-8000-00000000000b")}
	}

	return cases
}

func (tc routeValidationScopeCase) reader() *atomicTransactionBatchSettingsReader {
	settings := func(validates bool) mmodel.LedgerSettings {
		value := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
		value.Accounting.ValidateRoutes = validates

		return value
	}

	return &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
		tc.ledgerA: settings(tc.validatesA),
		tc.ledgerB: settings(tc.validatesB),
	}}
}

func (tc routeValidationScopeCase) parts(t *testing.T) []decomposedCrossLedgerPart {
	t.Helper()

	parts, err := decomposeCrossLedgerTransaction(crossLedgerTestTransaction("10",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "10", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "10", false)}),
		crossLedgerTransactionScopes{from: []atomicTransactionBatchLedgerRef{tc.ledgerA}, to: []atomicTransactionBatchLedgerRef{tc.ledgerB}})
	require.NoError(t, err)

	return parts
}

func requireRouteValidationScope(t *testing.T, tc routeValidationScopeCase, err error) {
	t.Helper()

	if !tc.wantRefused {
		require.NoError(t, err)
		return
	}

	var businessErr pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &businessErr)
	assert.Equal(t, constant.ErrCrossLedgerRouteValidationUnsupported.Error(), businessErr.Code)
}

func TestCrossLedgerGroupBatch_RefusesRouteValidationOnlyAcrossOrganizations(t *testing.T) {
	for _, tc := range routeValidationScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			uc := &UseCase{
				TransactionReader: tc.reader(),
				UUIDv7Generator:   func() (uuid.UUID, error) { return uuid.NewV7() },
				Clock:             func() time.Time { return time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC) },
			}

			_, err := uc.initializeAtomicTransactionBatchV2(context.Background(),
				buildCrossLedgerAtomicBatchInput(CreateCrossLedgerTransactionV2Input{}, uuid.New(), tc.parts(t)))

			requireRouteValidationScope(t, tc, err)
		})
	}
}

func TestValidateCrossLedgerHoldSettings_RefusesRouteValidationOnlyAcrossOrganizations(t *testing.T) {
	for _, tc := range routeValidationScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := buildCrossLedgerGroupIntent("BRL", tc.parts(t))
			require.NoError(t, err)

			err = (&UseCase{TransactionReader: tc.reader()}).validateCrossLedgerHoldSettings(context.Background(), intent)

			requireRouteValidationScope(t, tc, err)
		})
	}
}

// A group that spans organizations is refused before the request's transaction
// route is read: that route belongs to one organization and could not answer
// for the other.
func TestRouteCrossLedgerBridgeLegs_RefusesAGroupAcrossOrganizationsBeforeReadingTheRoute(t *testing.T) {
	for _, tc := range routeValidationScopeCases() {
		if !tc.wantRefused {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			routeID := uuid.MustParse("0199b800-0000-7000-8000-000000000003").String()
			request := crossLedgerTestTransaction("10",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "10", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "10", false)})
			request.RouteID = &routeID

			// The reader implements no route cache: a route read would fail with
			// a technical error instead of the refusal.
			err := (&UseCase{TransactionReader: tc.reader()}).routeCrossLedgerBridgeLegs(context.Background(), request, tc.parts(t))

			requireRouteValidationScope(t, tc, err)
		})
	}
}

// A group created while no participant validated routes meets the rule again at
// commit or cancel, where settings are read anew.
func TestTransitionCrossLedgerGroupV2_RefusesRouteValidationAcrossOrganizations(t *testing.T) {
	for _, status := range []string{constant.APPROVED, constant.CANCELED} {
		t.Run(status, func(t *testing.T) {
			validating := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
			validating.Accounting.ValidateRoutes = true
			otherOrganizationID := uuid.MustParse("0199b800-0000-7000-8000-000000000002")

			uc, repo, engine, target, in, group := newCrossLedgerLifecycleFixtureWith(t, status, crossLedgerLifecycleSetup{
				destinationOrganizationID: &otherOrganizationID,
				settingsB:                 &validating,
				refusedBeforeLocks:        true,
			})
			repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)

			_, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, status)

			var businessErr pkg.UnprocessableOperationError
			require.ErrorAs(t, err, &businessErr)
			assert.Equal(t, constant.ErrCrossLedgerRouteValidationUnsupported.Error(), businessErr.Code)
			assert.Empty(t, engine.executions, "a refused transition never reaches the engine")
		})
	}
}
