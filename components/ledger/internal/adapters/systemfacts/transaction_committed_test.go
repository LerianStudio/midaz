// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// The fixed identifiers every scenario is built around.
const (
	fixedOrganizationID = "0199c6f0-0000-7000-8000-00000000000a"
	fixedLedgerID       = "0199c6f0-0000-7000-8000-00000000000b"
	fixedTransactionID  = "0199c6f0-0000-7000-8000-000000000001"
	fixedParentID       = "0199c6f0-0000-7000-8000-000000000002"
	fixedGroupID        = "0199c6f0-0000-7000-8000-000000000003"
	externalAccountID   = "0199c6f0-0000-7000-8000-0000000000e1"
	aliceAccountID      = "0199c6f0-0000-7000-8000-0000000000a1"
	bobAccountID        = "0199c6f0-0000-7000-8000-0000000000a2"
	feeAccountID        = "0199c6f0-0000-7000-8000-0000000000f1"

	externalAlias = constant.DefaultExternalAccountAliasPrefix + "BRL"
	aliceAlias    = "@alice"
	bobAlias      = "@bob"
	feeAlias      = "@fees"

	// fixedCreatedAtWire is fixedCreatedAt rendered in UTC: the builder must
	// convert the row's zone, not copy it.
	fixedCreatedAtWire = "2026-10-09T15:00:00.123456789Z"
)

var (
	fixedCreatedAt = time.Date(2026, 10, 9, 12, 0, 0, 123456789, time.FixedZone("BRT", -3*60*60))
	fixedUpdatedAt = time.Date(2026, 10, 9, 15, 5, 0, 0, time.UTC)
)

func legID(n int) string {
	return fmt.Sprintf("0199c6f0-0000-7000-8000-0000000001%02x", n)
}

type legOption func(*operation.Operation)

func withChartOfAccounts(code string) legOption {
	return func(op *operation.Operation) { op.ChartOfAccounts = code }
}

func withMetadata(metadata map[string]any) legOption {
	return func(op *operation.Operation) { op.Metadata = metadata }
}

func withDirection(direction string) legOption {
	return func(op *operation.Operation) { op.Direction = direction }
}

// withoutDirection makes the row a legacy one, written before the direction
// column existed.
func withoutDirection() legOption {
	return withDirection("")
}

// leg builds a row the way the completion persists it: the side rides on
// Direction, which the engine sets to debit for a source leg and credit for a
// destination leg, whatever the row Type.
func leg(n int, opType, amount, accountID, alias string, opts ...legOption) *operation.Operation {
	value := decimal.RequireFromString(amount)

	direction := constant.DirectionDebit
	if opType == constant.CREDIT {
		direction = constant.DirectionCredit
	}

	op := &operation.Operation{
		ID:             legID(n),
		TransactionID:  fixedTransactionID,
		Type:           opType,
		Direction:      direction,
		AssetCode:      "BRL",
		Amount:         operation.Amount{Value: &value},
		AccountID:      accountID,
		AccountAlias:   alias,
		OrganizationID: fixedOrganizationID,
		LedgerID:       fixedLedgerID,
	}

	for _, opt := range opts {
		opt(op)
	}

	return op
}

func externalDebit(n int, amount string, opts ...legOption) *operation.Operation {
	return leg(n, constant.DEBIT, amount, externalAccountID, externalAlias, opts...)
}

func externalCredit(n int, amount string, opts ...legOption) *operation.Operation {
	return leg(n, constant.CREDIT, amount, externalAccountID, externalAlias, opts...)
}

func aliceDebit(n int, amount string, opts ...legOption) *operation.Operation {
	return leg(n, constant.DEBIT, amount, aliceAccountID, aliceAlias, opts...)
}

func bobCredit(n int, amount string, opts ...legOption) *operation.Operation {
	return leg(n, constant.CREDIT, amount, bobAccountID, bobAlias, opts...)
}

// aliceOnHoldSource is the source row of a route-validated commit: the engine
// persists the unreserve as an ON_HOLD-typed row on the debit side.
func aliceOnHoldSource(n int, amount string, opts ...legOption) *operation.Operation {
	return leg(n, constant.ONHOLD, amount, aliceAccountID, aliceAlias, opts...)
}

func feeCredit(n int, amount string, feeLegValue string, opts ...legOption) *operation.Operation {
	opts = append(opts, withMetadata(map[string]any{constant.MetadataKeyFeeLeg: feeLegValue}))

	return leg(n, constant.CREDIT, amount, feeAccountID, feeAlias, opts...)
}

type transactionOption func(*transaction.Transaction)

func withStatus(code string) transactionOption {
	return func(tx *transaction.Transaction) { tx.Status = transaction.Status{Code: code} }
}

func withAmount(amount string) transactionOption {
	return func(tx *transaction.Transaction) {
		value := decimal.RequireFromString(amount)
		tx.Amount = &value
	}
}

func withScheme(scheme string) transactionOption {
	return func(tx *transaction.Transaction) { tx.Scheme = scheme }
}

func withDescription(description string) transactionOption {
	return func(tx *transaction.Transaction) { tx.Description = description }
}

func withParent(id string) transactionOption {
	return func(tx *transaction.Transaction) { tx.ParentTransactionID = testutils.Ptr(id) }
}

func withGroup(id string) transactionOption {
	return func(tx *transaction.Transaction) { tx.GroupID = testutils.Ptr(id) }
}

func withChartGroup(name string) transactionOption {
	return func(tx *transaction.Transaction) { tx.ChartOfAccountsGroupName = name }
}

// writeSet builds the completion write set of an APPROVED BRL transaction of
// 150.00 with the given legs; opts override any of those defaults.
func writeSet(action string, legs []*operation.Operation, opts ...transactionOption) command.TransactionWriteSet {
	amount := decimal.RequireFromString("150.00")

	tx := &transaction.Transaction{
		ID:             fixedTransactionID,
		OrganizationID: fixedOrganizationID,
		LedgerID:       fixedLedgerID,
		Status:         transaction.Status{Code: constant.APPROVED},
		Amount:         &amount,
		AssetCode:      "BRL",
		CreatedAt:      fixedCreatedAt,
		UpdatedAt:      fixedUpdatedAt,
		Operations:     legs,
	}

	for _, opt := range opts {
		opt(tx)
	}

	return command.TransactionWriteSet{Transaction: tx, Action: action}
}

func wantLeg(n int, opType, amount, accountID string, external, feeLeg bool) TransactionCommittedOperation {
	return TransactionCommittedOperation{
		ID:        legID(n),
		Type:      opType,
		Amount:    amount,
		AssetCode: "BRL",
		AccountID: accountID,
		External:  external,
		FeeLeg:    feeLeg,
	}
}

func withWantChartOfAccounts(op TransactionCommittedOperation, code string) TransactionCommittedOperation {
	op.ChartOfAccounts = code

	return op
}

// payloadCase is one builder scenario. wantFact false means the write set is
// not a fact and want is ignored.
type payloadCase struct {
	name     string
	record   command.TransactionWriteSet
	wantFact bool
	want     TransactionCommittedPayload
}

// payloadCases is shared by the mapping test and the contract validation test
// so every fact scenario the builder produces is also proven against the
// embedded schema.
func payloadCases() []payloadCase {
	return []payloadCase{
		{
			name:     "inflow direct: external debit into an internal account",
			record:   writeSet(constant.ActionDirect, []*operation.Operation{externalDebit(1, "150.00"), bobCredit(2, "150.00")}),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "INFLOW",
				Amount:    "150",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "150", externalAccountID, true, false),
					wantLeg(2, constant.CREDIT, "150", bobAccountID, false, false),
				},
			},
		},
		{
			name: "outflow direct: internal debit to the external counterparty",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{
					aliceDebit(1, "1250.75", withChartOfAccounts("1101")),
					externalCredit(2, "1250.75", withChartOfAccounts("2101")),
				},
				withAmount("1250.75"), withChartGroup("SETTLEMENTS")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:                   constant.ActionDirect,
				Status:                   constant.APPROVED,
				Direction:                "OUTFLOW",
				Amount:                   "1250.75",
				AssetCode:                "BRL",
				ChartOfAccountsGroupName: "SETTLEMENTS",
				CreatedAt:                fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					withWantChartOfAccounts(wantLeg(1, constant.DEBIT, "1250.75", aliceAccountID, false, false), "1101"),
					withWantChartOfAccounts(wantLeg(2, constant.CREDIT, "1250.75", externalAccountID, true, false), "2101"),
				},
			},
		},
		{
			name: "outflow commit with scheme and description",
			record: writeSet(constant.ActionCommit,
				[]*operation.Operation{aliceDebit(1, "89.90"), externalCredit(2, "89.90")},
				withAmount("89.90"), withScheme("PIX"), withDescription("Pix payment, order 48213")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:      constant.ActionCommit,
				Status:      constant.APPROVED,
				Direction:   "OUTFLOW",
				Amount:      "89.9",
				AssetCode:   "BRL",
				Scheme:      "PIX",
				Description: "Pix payment, order 48213",
				CreatedAt:   fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "89.9", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "89.9", externalAccountID, true, false),
				},
			},
		},
		{
			name: "revert carries the parent transaction id",
			record: writeSet(constant.ActionRevert,
				[]*operation.Operation{bobCredit(1, "150.00"), externalDebit(2, "150.00")},
				withParent(fixedParentID)),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:              constant.ActionRevert,
				Status:              constant.APPROVED,
				Direction:           "INFLOW",
				Amount:              "150",
				AssetCode:           "BRL",
				ParentTransactionID: fixedParentID,
				CreatedAt:           fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.CREDIT, "150", bobAccountID, false, false),
					wantLeg(2, constant.DEBIT, "150", externalAccountID, true, false),
				},
			},
		},
		{
			name: "internal with a fee leg marked by the string true",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{
					aliceDebit(1, "1000.00", withChartOfAccounts("1102")),
					bobCredit(2, "985.00", withChartOfAccounts("2102")),
					feeCredit(3, "15.00", constant.MetadataValueFeeLeg, withChartOfAccounts("4101")),
				},
				withAmount("1000.00"), withChartGroup("CARD_PURCHASES"), withScheme("CARD"), withGroup(fixedGroupID)),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:                   constant.ActionDirect,
				Status:                   constant.APPROVED,
				Direction:                "INTERNAL",
				Amount:                   "1000",
				AssetCode:                "BRL",
				ChartOfAccountsGroupName: "CARD_PURCHASES",
				Scheme:                   "CARD",
				GroupID:                  fixedGroupID,
				CreatedAt:                fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					withWantChartOfAccounts(wantLeg(1, constant.DEBIT, "1000", aliceAccountID, false, false), "1102"),
					withWantChartOfAccounts(wantLeg(2, constant.CREDIT, "985", bobAccountID, false, false), "2102"),
					withWantChartOfAccounts(wantLeg(3, constant.CREDIT, "15", feeAccountID, false, true), "4101"),
				},
			},
		},
		{
			name: "fee mark with any other value is not a fee leg",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{aliceDebit(1, "150.00"), feeCredit(2, "150.00", "yes")}),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "INTERNAL",
				Amount:    "150",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "150", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "150", feeAccountID, false, false),
				},
			},
		},
		{
			name: "route-validated commit: on-hold source leg to the external counterparty",
			record: writeSet(constant.ActionCommit,
				[]*operation.Operation{aliceOnHoldSource(1, "89.90"), externalCredit(2, "89.90")},
				withAmount("89.90"), withScheme("PIX")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionCommit,
				Status:    constant.APPROVED,
				Direction: "OUTFLOW",
				Amount:    "89.9",
				AssetCode: "BRL",
				Scheme:    "PIX",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.ONHOLD, "89.9", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "89.9", externalAccountID, true, false),
				},
			},
		},
		{
			name: "outflow with a fee leg: the fee engine's internal credit does not mask the flow",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{
					aliceDebit(1, "1000.00"),
					externalCredit(2, "985.00"),
					feeCredit(3, "15.00", constant.MetadataValueFeeLeg),
				},
				withAmount("1000.00"), withScheme("PIX")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "OUTFLOW",
				Amount:    "1000",
				AssetCode: "BRL",
				Scheme:    "PIX",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "1000", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "985", externalAccountID, true, false),
					wantLeg(3, constant.CREDIT, "15", feeAccountID, false, true),
				},
			},
		},
		{
			name: "inflow with a fee leg stays an inflow",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{
					externalDebit(1, "1000.00"),
					bobCredit(2, "985.00"),
					feeCredit(3, "15.00", constant.MetadataValueFeeLeg),
				},
				withAmount("1000.00")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "INFLOW",
				Amount:    "1000",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "1000", externalAccountID, true, false),
					wantLeg(2, constant.CREDIT, "985", bobAccountID, false, false),
					wantLeg(3, constant.CREDIT, "15", feeAccountID, false, true),
				},
			},
		},
		{
			name: "legacy rows without a direction fall back to the row type",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{
					externalDebit(1, "150.00", withoutDirection()),
					bobCredit(2, "150.00", withoutDirection()),
				}),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "INFLOW",
				Amount:    "150",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "150", externalAccountID, true, false),
					wantLeg(2, constant.CREDIT, "150", bobAccountID, false, false),
				},
			},
		},
		{
			name: "a legacy on-hold row without a direction is not counted",
			record: writeSet(constant.ActionCommit,
				[]*operation.Operation{
					aliceOnHoldSource(1, "89.90", withoutDirection()),
					externalCredit(2, "89.90", withoutDirection()),
				},
				withAmount("89.90")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionCommit,
				Status:    constant.APPROVED,
				Direction: "MIXED",
				Amount:    "89.9",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.ONHOLD, "89.9", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "89.9", externalAccountID, true, false),
				},
			},
		},
		{
			name: "mixed: external debit split between an internal and an external credit",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{externalDebit(1, "500"), bobCredit(2, "300"), externalCredit(3, "200")},
				withAmount("500")),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "MIXED",
				Amount:    "500",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "500", externalAccountID, true, false),
					wantLeg(2, constant.CREDIT, "300", bobAccountID, false, false),
					wantLeg(3, constant.CREDIT, "200", externalAccountID, true, false),
				},
			},
		},
		{
			name:     "no external leg is internal",
			record:   writeSet(constant.ActionDirect, []*operation.Operation{aliceDebit(1, "150.00"), bobCredit(2, "150.00")}),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "INTERNAL",
				Amount:    "150",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.DEBIT, "150", aliceAccountID, false, false),
					wantLeg(2, constant.CREDIT, "150", bobAccountID, false, false),
				},
			},
		},
		{
			name: "credit-only legs with an external one are mixed",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{externalCredit(1, "150.00"), bobCredit(2, "150.00")}),
			wantFact: true,
			want: TransactionCommittedPayload{
				Action:    constant.ActionDirect,
				Status:    constant.APPROVED,
				Direction: "MIXED",
				Amount:    "150",
				AssetCode: "BRL",
				CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					wantLeg(1, constant.CREDIT, "150", externalAccountID, true, false),
					wantLeg(2, constant.CREDIT, "150", bobAccountID, false, false),
				},
			},
		},
		{
			name: "hold is not a fact",
			record: writeSet(constant.ActionHold,
				[]*operation.Operation{aliceDebit(1, "150.00"), externalCredit(2, "150.00")},
				withStatus(constant.PENDING)),
		},
		{
			name: "cancel is not a fact",
			record: writeSet(constant.ActionCancel,
				[]*operation.Operation{aliceDebit(1, "150.00"), externalCredit(2, "150.00")},
				withStatus(constant.CANCELED)),
		},
		{
			name: "a pending row under a direct action is not a fact",
			record: writeSet(constant.ActionDirect,
				[]*operation.Operation{aliceDebit(1, "150.00"), externalCredit(2, "150.00")},
				withStatus(constant.PENDING)),
		},
		{
			name:   "nil transaction is not a fact",
			record: command.TransactionWriteSet{Action: constant.ActionDirect},
		},
	}
}

func TestNewTransactionCommitted(t *testing.T) {
	t.Parallel()

	for _, tc := range payloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, isFact := NewTransactionCommitted(tc.record)

			require.Equal(t, tc.wantFact, isFact)

			if !tc.wantFact {
				assert.Equal(t, TransactionCommittedPayload{}, got, "a write set that is not a fact derives nothing")

				return
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNewTransactionCommitted_PreservesOperationOrder(t *testing.T) {
	t.Parallel()

	record := writeSet(constant.ActionDirect, []*operation.Operation{
		bobCredit(3, "100"),
		externalDebit(1, "150"),
		bobCredit(2, "50"),
	})

	got, isFact := NewTransactionCommitted(record)
	require.True(t, isFact)

	ids := make([]string, 0, len(got.Operations))
	for _, op := range got.Operations {
		ids = append(ids, op.ID)
	}

	assert.Equal(t, []string{legID(3), legID(1), legID(2)}, ids)
}

func jsonKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()

	var object map[string]json.RawMessage

	require.NoError(t, json.Unmarshal(raw, &object))

	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}

	return keys
}

func TestTransactionCommittedPayload_JSONShape(t *testing.T) {
	t.Parallel()

	requiredRoot := []string{"action", "status", "direction", "amount", "assetCode", "createdAt", "operations"}
	requiredLeg := []string{"id", "type", "amount", "assetCode", "accountId", "external", "feeLeg"}

	tests := []struct {
		name     string
		payload  TransactionCommittedPayload
		wantRoot []string
		wantLeg  []string
	}{
		{
			name: "required fields only",
			payload: TransactionCommittedPayload{
				Action: constant.ActionDirect, Status: constant.APPROVED, Direction: "INFLOW",
				Amount: "150", AssetCode: "BRL", CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{wantLeg(1, constant.DEBIT, "150", externalAccountID, true, false)},
			},
			wantRoot: requiredRoot,
			wantLeg:  requiredLeg,
		},
		{
			name: "every optional field",
			payload: TransactionCommittedPayload{
				Action: constant.ActionRevert, Status: constant.APPROVED, Direction: "OUTFLOW",
				Amount: "150", AssetCode: "BRL", ChartOfAccountsGroupName: "SETTLEMENTS", Scheme: "PIX",
				Description: "d", ParentTransactionID: fixedParentID, GroupID: fixedGroupID, CreatedAt: fixedCreatedAtWire,
				Operations: []TransactionCommittedOperation{
					withWantChartOfAccounts(wantLeg(1, constant.DEBIT, "150", aliceAccountID, false, false), "1101"),
				},
			},
			wantRoot: append(append([]string{}, requiredRoot...), "chartOfAccountsGroupName", "scheme", "description", "parentTransactionId", "groupId"),
			wantLeg:  append(append([]string{}, requiredLeg...), "chartOfAccounts"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			assert.ElementsMatch(t, tc.wantRoot, jsonKeys(t, raw))

			var root struct {
				Operations []json.RawMessage `json:"operations"`
			}

			require.NoError(t, json.Unmarshal(raw, &root))
			require.Len(t, root.Operations, 1)
			assert.ElementsMatch(t, tc.wantLeg, jsonKeys(t, root.Operations[0]))
		})
	}
}
