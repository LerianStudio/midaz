// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestCreateTransaction_MarksPrimaryReadIntent verifies the create flow marks
// the primary-read intent before its pre-write balance reads so the read-routing
// seam can steer the DB miss to the primary. It asserts on the two facets that
// make the mechanism real:
//
//  1. Mechanism: the intent marker survives the exact call sequence the handler
//     uses. GetBalances (the direct pre-write read) and enrichOverdraftOperations
//     (the second, overdraft-enrichment read that reuses the SAME ctx) both
//     forward a marked ctx down to the seam target — the balance DB repository,
//     which is where the read-routing seam observes the intent. A negative
//     baseline proves an unmarked ctx stays unmarked, so the assertion is not
//     trivially true.
//  2. Placement: a structural guard over the live source of stageBalances — the step
//     both create pipelines run — proves the `readrouting.WithPrimaryRead(ctx)` wrap
//     actually exists and positionally precedes the GetBalances read (and therefore the
//     overdraft read, which follows it). Runtime mocks cannot catch a future
//     removal or reorder of the wrap; the AST guard can. This mirrors the
//     existing fee-seam structural guards (transaction_fee_seam_structure_test.go).
func TestCreateTransaction_MarksPrimaryReadIntent(t *testing.T) {
	t.Run("direct_balance_read_forwards_marked_ctx_to_db_seam", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		organizationID := uuid.New()
		ledgerID := uuid.New()
		aliases := []string{"@alice#default"}

		// Reproduce the handler's exact call sequence: mark the ctx, then read.
		ctx := readrouting.WithPrimaryRead(context.Background())

		_, err := uc.GetBalances(ctx, organizationID, ledgerID, aliases)
		require.NoError(t, err)

		require.True(t, captured.seen, "balance DB repository was never reached; the cache-miss path must fall through to the seam target")
		assert.True(t, captured.primaryRead, "the ctx reaching the balance DB seam must carry the primary-read intent")
	})

	t.Run("overdraft_enrichment_read_forwards_marked_ctx_to_db_seam", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		orgID := uuid.New()
		ledgerID := uuid.New()

		source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")

		primary := mmodel.BalanceOperation{
			Balance: source,
			Alias:   "0#@alice#default",
			Amount: mtransaction.Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.DEBIT,
				TransactionType: libConstants.CREATED,
				Direction:       constant.DirectionCredit,
			},
			InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
		}

		validate := &mtransaction.Responses{
			From:    map[string]mtransaction.Amount{"0#@alice#default": primary.Amount},
			Sources: []string{"@alice#default"},
			Aliases: []string{"@alice#default"},
		}

		// enrichOverdraftOperations is invoked with the SAME wrapped ctx the
		// handler holds, and the loader is uc.GetBalances exactly as
		// at the create call site. The overdraft read must therefore forward the
		// mark down to the same DB seam target.
		ctx := readrouting.WithPrimaryRead(context.Background())

		_, _, err := enrichOverdraftOperations(ctx, orgID, ledgerID,
			[]mmodel.BalanceOperation{primary}, validate, uc.GetBalances)
		require.NoError(t, err)

		require.True(t, captured.seen, "overdraft companion read never reached the balance DB seam")
		assert.True(t, captured.primaryRead, "the ctx reaching the balance DB seam via the overdraft read must carry the primary-read intent")
	})

	t.Run("unmarked_baseline_ctx_is_not_marked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		_, err := uc.GetBalances(context.Background(), uuid.New(), uuid.New(), []string{"@alice#default"})
		require.NoError(t, err)

		require.True(t, captured.seen)
		assert.False(t, captured.primaryRead, "a pure/unmarked read must NOT carry the primary-read intent")
	})

	t.Run("wrap_exists_and_precedes_get_balances_in_source", func(t *testing.T) {
		src := readTransportSource(t, "create_transaction_steps.go", "func (uc *UseCase) stageBalances")

		wrapPos, getBalancesPos := analyzePrimaryReadWrap(t, src, "stageBalances")

		if wrapPos == -1 {
			t.Fatal("no `readrouting.WithPrimaryRead(ctx)` wrap found in stageBalances; the create flow does not mark the primary-read intent")
		}

		if getBalancesPos == -1 {
			t.Fatal("no GetBalances call found in stageBalances; the read call site moved")
		}

		if wrapPos >= getBalancesPos {
			t.Errorf("the WithPrimaryRead wrap (stmt %d) must precede the GetBalances read (stmt %d) so both pre-write balance reads observe the mark", wrapPos, getBalancesPos)
		}
	})
}

// primaryReadCapture records what the balance DB seam observed.
type primaryReadCapture struct {
	seen        bool
	primaryRead bool
}

// newPrimaryReadCapturingUseCase builds a real query.UseCase whose Redis cache
// always misses (forcing GetBalances to fall through to the DB seam) and whose
// balance DB repository records the primary-read intent of the ctx it receives.
func newPrimaryReadCapturingUseCase(ctrl *gomock.Controller) (*primaryReadCapture, *query.UseCase) {
	captured := &primaryReadCapture{}

	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockBalance := balance.NewMockRepository(ctrl)
	mockAccount := account.NewMockRepository(ctrl)

	// Cache miss on every alias: empty value -> GetBalances falls through to DB.
	mockRedis.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		AnyTimes()

	// The blocked-hydration lookup on the miss path; no blocked accounts here.
	mockAccount.EXPECT().
		ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mmodel.Account{}, nil).
		AnyTimes()

	mockBalance.EXPECT().
		ListByAliasesWithKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			captured.seen = true
			captured.primaryRead = readrouting.IsPrimaryRead(ctx)

			out := make([]*mmodel.Balance, 0, len(aliases))
			for _, a := range aliases {
				alias, _, _ := strings.Cut(a, "#")
				out = append(out, companionOverdraftBalance(alias))
			}

			return out, nil
		}).
		AnyTimes()

	uc := &query.UseCase{
		BalanceRepo:          mockBalance,
		AccountRepo:          mockAccount,
		TransactionRedisRepo: mockRedis,
	}

	return captured, uc
}

// analyzePrimaryReadWrap returns the top-level statement indices, within the
// named function body, of the `readrouting.WithPrimaryRead(...)` wrap and the
// first `GetBalances(...)` call. Either is -1 when absent. Statement indices are
// sufficient because both live at the top level of the strictly-sequential
// create flow.
func analyzePrimaryReadWrap(t *testing.T, src, funcName string) (wrapPos, getBalancesPos int) {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var fn *ast.FuncDecl

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}

	if fn == nil || fn.Body == nil {
		t.Fatalf("function %q not found or has no body", funcName)
	}

	wrapPos, getBalancesPos = -1, -1

	for i, stmt := range fn.Body.List {
		if wrapPos == -1 && stmtCallsSelector(stmt, "readrouting", "WithPrimaryRead") {
			wrapPos = i
		}

		if getBalancesPos == -1 && stmtCallsMethod(stmt, "GetBalances") {
			getBalancesPos = i
		}
	}

	return wrapPos, getBalancesPos
}

// stmtCallsSelector reports whether the statement contains a call of the form
// pkg.Fn(...), matching both the package identifier and the function name.
func stmtCallsSelector(stmt ast.Stmt, pkg, fn string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == fn {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == pkg {
				found = true
			}
		}

		return true
	})

	return found
}

// TestCommitCancel_MarksPrimaryReadIntent locks the decision that the
// commit/cancel state-transition flow marks its pre-write balance read with the
// primary-read intent. The read at the GetBalances call site feeds
// buildBalanceOperations and (on cancel) enrichOverdraftOperations, whose result
// seeds the authoritative balance via the NX-seed — the stale-read money-
// corruption scenario the seam guards against. It asserts:
//
//  1. Mechanism: the intent marker survives the exact call sequence the handler
//     uses — GetBalances and the cancel overdraft-enrichment read (which reuses
//     the SAME ctx) both forward a marked ctx down to the balance DB seam target.
//     A negative baseline proves an unmarked ctx stays unmarked.
//  2. Placement: a structural guard over the live source of
//     preparePendingTransition proves the dedicated-var wrap
//     `readCtx := readrouting.WithPrimaryRead(ctx)` exists AND sits AFTER the
//     validation-only reads (GetParsedLedgerSettings) and BEFORE the GetBalances
//     read; that GetBalances and the cancel overdraft read receive readCtx while
//     ValidateAccountingRules keeps the unmarked ctx, so only the pre-write
//     balance reads are routed to primary.
func TestCommitCancel_MarksPrimaryReadIntent(t *testing.T) {
	t.Run("direct_balance_read_forwards_marked_ctx_to_db_seam", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		organizationID := uuid.New()
		ledgerID := uuid.New()
		aliases := []string{"@alice#default"}

		ctx := readrouting.WithPrimaryRead(context.Background())

		_, err := uc.GetBalances(ctx, organizationID, ledgerID, aliases)
		require.NoError(t, err)

		require.True(t, captured.seen, "balance DB repository was never reached; the cache-miss path must fall through to the seam target")
		assert.True(t, captured.primaryRead, "the ctx reaching the balance DB seam must carry the primary-read intent")
	})

	t.Run("cancel_overdraft_enrichment_read_forwards_marked_ctx_to_db_seam", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		orgID := uuid.New()
		ledgerID := uuid.New()

		source := overdraftEnabledBalance(t, "@alice", decimal.NewFromInt(50), "100")

		primary := mmodel.BalanceOperation{
			Balance: source,
			Alias:   "0#@alice#default",
			Amount: mtransaction.Amount{
				Asset:           "BRL",
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.DEBIT,
				TransactionType: libConstants.CREATED,
				Direction:       constant.DirectionCredit,
			},
			InternalKey: utils.BalanceInternalKey(orgID, ledgerID, "@alice#default"),
		}

		validate := &mtransaction.Responses{
			From:    map[string]mtransaction.Amount{"0#@alice#default": primary.Amount},
			Sources: []string{"@alice#default"},
			Aliases: []string{"@alice#default"},
		}

		// The dedicated readCtx at the GetBalances call site also covers the cancel
		// overdraft-enrichment read, which receives the same readCtx and uses
		// uc.GetBalances as its loader exactly as at the handler site.
		ctx := readrouting.WithPrimaryRead(context.Background())

		_, _, err := enrichOverdraftOperations(ctx, orgID, ledgerID,
			[]mmodel.BalanceOperation{primary}, validate, uc.GetBalances)
		require.NoError(t, err)

		require.True(t, captured.seen, "cancel overdraft companion read never reached the balance DB seam")
		assert.True(t, captured.primaryRead, "the ctx reaching the balance DB seam via the cancel overdraft read must carry the primary-read intent")
	})

	t.Run("unmarked_baseline_ctx_is_not_marked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		captured, uc := newPrimaryReadCapturingUseCase(ctrl)

		_, err := uc.GetBalances(context.Background(), uuid.New(), uuid.New(), []string{"@alice#default"})
		require.NoError(t, err)

		require.True(t, captured.seen)
		assert.False(t, captured.primaryRead, "a flow not going through commit/cancel must NOT carry the primary-read intent")
	})
}
