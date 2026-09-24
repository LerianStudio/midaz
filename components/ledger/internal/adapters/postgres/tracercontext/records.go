// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracercontext reads official Ledger records for Tracer projections.
package tracercontext

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// Repository uses a read-only repeatable-read transaction on the tenant primary.
// It never reads replicas or invents a missing asset identity from its code.
type Repository struct {
	connection    *libPostgres.Client
	bounds        tracercontract.Limits
	requireTenant bool
}

func NewRepository(connection *libPostgres.Client, bounds tracercontract.Limits, requireTenant bool) (*Repository, error) {
	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	return &Repository{connection: connection, bounds: bounds, requireTenant: requireTenant}, nil
}

// Read returns exactly the requested accounts and their assets plus assets named
// by entryCodes (including external entries). Empty accounts are allowed only
// with entry codes. Callers must authorize the scope and apply off/skip gates
// before reading. A snapshot does not freeze facts against subsequent updates.
func (r *Repository) Read(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID, entryCodes []string) (_ []*mmodel.Account, _ []*mmodel.Asset, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.read_tracer_records")
	defer span.End()
	defer func() {
		if retErr == nil {
			return
		}

		if errors.Is(retErr, constant.ErrInvalidRequestBody) || pkg.IsBusinessError(retErr) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Official record request rejected", retErr)
		} else {
			libOtel.HandleSpanError(span, "Official record snapshot failed", retErr)
		}
	}()

	if err := r.validateRequest(organizationID, ledgerID, accountIDs, entryCodes); err != nil {
		return nil, nil, err
	}

	db, err := r.database(ctx)
	if err != nil {
		return nil, nil, err
	}
	// dbresolver.BeginTx always selects ReadWrite, even with ReadOnly=true.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("begin official record snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	accounts, err := r.readAccounts(ctx, tx, organizationID, ledgerID, accountIDs)
	if err != nil {
		return nil, nil, err
	}

	codes := make(map[string]struct{}, len(entryCodes))
	for _, code := range entryCodes {
		codes[code] = struct{}{}
	}

	for _, account := range accounts {
		codes[account.AssetCode] = struct{}{}
	}

	assets, err := r.readAssets(ctx, tx, organizationID, ledgerID, codes)
	if err != nil {
		return nil, nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("finish official record snapshot: %w", err)
	}

	return accounts, assets, nil
}

func (r *Repository) validateRequest(org, ledger uuid.UUID, ids []uuid.UUID, codes []string) error {
	if org == uuid.Nil || ledger == uuid.Nil || (len(ids) == 0 && len(codes) == 0) || len(ids) > r.bounds.MaxAccounts || len(codes) > r.bounds.MaxEntries {
		return constant.ErrInvalidRequestBody
	}

	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists || id == uuid.Nil {
			return constant.ErrInvalidRequestBody
		}

		seen[id] = struct{}{}
	}

	for _, code := range codes {
		if !r.validText(code) {
			return constant.ErrInvalidRequestBody
		}
	}

	return nil
}

func (r *Repository) validText(value string) bool {
	return value != "" && len(value) <= r.bounds.MaxTextBytes && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func (r *Repository) database(ctx context.Context) (dbresolver.DB, error) {
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant || r.connection == nil {
		return nil, constant.ErrInternalServer
	}

	return r.connection.Resolver(ctx)
}

func (r *Repository) readAccounts(ctx context.Context, tx dbresolver.Tx, org, ledger uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	if len(ids) == 0 {
		return []*mmodel.Account{}, nil
	}
	// Bound text before it crosses the database boundary; NULL fails closed at Scan.
	rows, err := tx.QueryContext(ctx, `SELECT id,
 CASE WHEN octet_length(asset_code)<=$4 THEN asset_code END,
 CASE WHEN octet_length(type)<=$4 THEN type END,
 CASE WHEN octet_length(status)<=$4 THEN status END, blocked
 FROM account WHERE organization_id=$1 AND ledger_id=$2 AND id=ANY($3)
 AND deleted_at IS NULL ORDER BY id LIMIT $5`, org, ledger, pq.Array(ids), r.bounds.MaxTextBytes, len(ids))
	if err != nil {
		return nil, fmt.Errorf("read official accounts: %w", err)
	}
	defer rows.Close()

	result := make([]*mmodel.Account, 0, len(ids))

	requested := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		requested[id] = true
	}

	for rows.Next() {
		var id uuid.UUID

		account := &mmodel.Account{OrganizationID: org.String(), LedgerID: ledger.String()}

		var blocked bool
		if err := rows.Scan(&id, &account.AssetCode, &account.Type, &account.Status.Code, &blocked); err != nil {
			return nil, fmt.Errorf("scan official account: %w", err)
		}

		if !requested[id] || !r.validText(account.AssetCode) || !r.validText(account.Type) || !r.validText(account.Status.Code) {
			return nil, constant.ErrTracerFactsUnavailable
		}

		delete(requested, id)
		account.ID = id.String()
		account.Blocked = &blocked
		result = append(result, account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate official accounts: %w", err)
	}

	if len(requested) != 0 {
		return nil, constant.ErrTracerFactsUnavailable
	}

	return result, nil
}

func (r *Repository) readAssets(ctx context.Context, tx dbresolver.Tx, org, ledger uuid.UUID, codes map[string]struct{}) ([]*mmodel.Asset, error) {
	ordered := make([]string, 0, len(codes))
	for code := range codes {
		ordered = append(ordered, code)
	}

	slices.Sort(ordered)
	// One extra row detects ambiguous codes, without transferring an unbounded set.
	rows, err := tx.QueryContext(ctx, `SELECT id, CASE WHEN octet_length(code)<=$4 THEN code END
 FROM asset WHERE organization_id=$1 AND ledger_id=$2 AND code=ANY($3)
 AND deleted_at IS NULL ORDER BY id LIMIT $5`, org, ledger, pq.Array(ordered), r.bounds.MaxTextBytes, int64(len(ordered))+1)
	if err != nil {
		return nil, fmt.Errorf("read official assets: %w", err)
	}
	defer rows.Close()

	result := make([]*mmodel.Asset, 0, len(codes))

	for rows.Next() {
		var id uuid.UUID

		asset := &mmodel.Asset{OrganizationID: org.String(), LedgerID: ledger.String()}
		if err := rows.Scan(&id, &asset.Code); err != nil {
			return nil, fmt.Errorf("scan official asset: %w", err)
		}

		if _, exists := codes[asset.Code]; !exists || id == uuid.Nil || !r.validText(asset.Code) {
			return nil, constant.ErrTracerFactsUnavailable
		}

		delete(codes, asset.Code)
		asset.ID = id.String()
		result = append(result, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate official assets: %w", err)
	}

	if len(codes) != 0 {
		return nil, constant.ErrTracerFactsUnavailable
	}

	return result, nil
}
