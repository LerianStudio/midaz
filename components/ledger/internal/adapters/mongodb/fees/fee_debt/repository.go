// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee_debt

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	cn "github.com/LerianStudio/lib-commons/v7/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"

	mmongoDB "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// ListQuery pages one ledger's debts oldest first: by debt id, which starts with its
// origin transaction id (a UUIDv7), or by seq, the engine's settlement order, within one debtor.
type ListQuery struct {
	// DebtorBalanceRef ("alias#key") narrows the listing to one debtor; empty lists all.
	DebtorBalanceRef string
	Limit            int
	Cursor           string
}

// Repository is the Fees record of fee debts, one document per debt, and the
// command.FeeDebtRecorder completion calls with each applied result's changes.
type Repository struct {
	connection *mmongoDB.MongoConnection
	database   string
	tenantDB   command.TenantMongoResolver
}

// NewRepository ensures the listing indexes on the static fee database. tenantDB is
// nil in single-tenant mode; otherwise every read and write goes to the database it
// resolves for the tenant on ctx, never to the static one.
func NewRepository(mc *mmongoDB.MongoConnection, tenantDB command.TenantMongoResolver) (*Repository, error) {
	if err := EnsureIndexes(context.Background(), mc); err != nil {
		return nil, fmt.Errorf("ensure fee debt indexes: %w", err)
	}

	return &Repository{connection: mc, database: strings.ToLower(mc.Database), tenantDB: tenantDB}, nil
}

// Apply records each change once per entry key: a replay is a no-op, and a debt's
// changes converge in any order because the first one seeds remaining with the opened
// amount, the rest move it by commuting increments and entries stay sorted.
func (r *Repository) Apply(ctx context.Context, record command.FeeDebtRecord) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.fee_debt.apply")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", record.OrganizationID.String()),
		attribute.String("app.request.ledger_id", record.LedgerID.String()),
		attribute.Int("app.request.fee_debt_changes", len(record.Changes)),
	)

	if len(record.Changes) == 0 {
		return nil
	}

	coll, err := r.collection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve fee debt database", err)

		return err
	}

	for _, change := range record.Changes {
		seed, filter, update, err := changeUpdate(record, change)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid fee debt change", err)

			return err
		}

		_, spanUpsert := tracer.Start(ctx, "repository.fee_debt.apply.upsert")

		if err := applyChange(ctx, coll, change.DebtID, seed, filter, update); err != nil {
			libOpentelemetry.HandleSpanError(spanUpsert, "Failed to record fee debt change", err)
			spanUpsert.End()

			return fmt.Errorf("record fee debt change: %w", err)
		}

		spanUpsert.End()
	}

	return nil
}

// FindByID returns the ledger's debt with id, or mongo.ErrNoDocuments.
func (r *Repository) FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id string) (*model.FeeDebt, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.fee_debt.find_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.fee_debt_id", id),
	)

	coll, err := r.collection(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve fee debt database", err)

		return nil, err
	}

	_, spanFind := tracer.Start(ctx, "repository.fee_debt.find_by_id.find_one")
	defer spanFind.End()

	var debt model.FeeDebt

	err = coll.FindOne(ctx, bson.D{
		{Key: "_id", Value: id},
		{Key: "organization_id", Value: organizationID.String()},
		{Key: "ledger_id", Value: ledgerID.String()},
	}).Decode(&debt)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanFind, "Fee debt not found", err)
		} else {
			libOpentelemetry.HandleSpanError(spanFind, "Failed to find fee debt", err)
		}

		return nil, err
	}

	return &debt, nil
}

// FindAll pages the ledger's debts oldest first. A cursor that does not decode is
// reported wrapping libHTTP.ErrInvalidCursor or libHTTP.ErrInvalidCursorDirection.
func (r *Repository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, query ListQuery) ([]*model.FeeDebt, libHTTP.CursorPagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.fee_debt.find_all")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Bool("app.request.filter_debtor", query.DebtorBalanceRef != ""),
		attribute.Int("app.request.limit", query.Limit),
	)

	filter := bson.D{
		{Key: "organization_id", Value: organizationID.String()},
		{Key: "ledger_id", Value: ledgerID.String()},
	}

	if query.DebtorBalanceRef != "" {
		filter = append(filter, bson.E{Key: "debtor_balance_ref", Value: query.DebtorBalanceRef})
	}

	key := "_id"
	if query.DebtorBalanceRef != "" {
		key = "seq"
	}

	isFirstPage := query.Cursor == ""
	direction := libHTTP.CursorDirectionNext
	sortOrder := 1

	if !isFirstPage {
		bound, cursorDirection, order, err := cursorBound(query.Cursor, key)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid fee debt cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		filter = append(filter, bound)
		direction, sortOrder = cursorDirection, order
	}

	debts, err := r.find(ctx, filter, bson.D{{Key: key, Value: sortOrder}}, query.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list fee debts", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasMore := len(debts) > query.Limit
	debts = libHTTP.PaginateRecords(isFirstPage, hasMore, direction, debts, query.Limit)

	var pagination libHTTP.CursorPagination

	if len(debts) > 0 {
		pagination, err = libHTTP.CalculateCursor(isFirstPage, hasMore, direction, position(key, debts[0]), position(key, debts[len(debts)-1]))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate fee debt cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	span.SetAttributes(attribute.Int("db.rows_returned", len(debts)))

	return debts, pagination, nil
}

// find reads up to limit+1 debts in sort order, the extra one telling whether more remain.
func (r *Repository) find(ctx context.Context, filter, sort bson.D, limit int) ([]*model.FeeDebt, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	coll, err := r.collection(ctx)
	if err != nil {
		return nil, err
	}

	_, spanFind := tracer.Start(ctx, "repository.fee_debt.find_all.find")
	defer spanFind.End()

	opts := options.Find().SetSort(sort).SetLimit(int64(limit) + 1)

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find fee debts", err)

		return nil, err
	}

	debts := make([]*model.FeeDebt, 0, limit+1)
	if err := cur.All(ctx, &debts); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to decode fee debts", err)

		return nil, err
	}

	return debts, nil
}

// collection is the fee_debt collection of the tenant's fee database in multi-tenant
// mode, resolved from the tenant on ctx and never taken from ctx, because a completion
// context carries another store under the generic key; of the static one otherwise.
func (r *Repository) collection(ctx context.Context) (*mongo.Collection, error) {
	if r.tenantDB != nil {
		tenantCtx, err := command.ResolveTenantMongoContext(ctx, r.tenantDB, "fee debt")
		if err != nil {
			return nil, err
		}

		return tmcore.GetMBContext(tenantCtx).Collection(feeconstant.FeeDebtCollection), nil
	}

	client, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	return client.Database(r.database).Collection(feeconstant.FeeDebtCollection), nil
}

// cursorBound decodes a listing cursor into the keyset bound on key, the page direction
// and the sort order. A seq cursor that is not a number is libHTTP.ErrInvalidCursor.
func cursorBound(cursor, key string) (bson.E, string, int, error) {
	decoded, err := libHTTP.DecodeCursor(cursor)
	if err != nil {
		return bson.E{}, "", 0, err
	}

	operator, order, err := libHTTP.CursorDirectionRules(cn.SortDirASC, decoded.Direction)
	if err != nil {
		return bson.E{}, "", 0, err
	}

	var value any = decoded.ID

	if key == "seq" {
		if value, err = strconv.ParseInt(decoded.ID, 10, 64); err != nil {
			return bson.E{}, "", 0, fmt.Errorf("%w: seq: %w", libHTTP.ErrInvalidCursor, err)
		}
	}

	mongoOperator, sortOrder := "$gt", 1
	if operator == "<" {
		mongoOperator = "$lt"
	}

	if order != cn.SortDirASC {
		sortOrder = -1
	}

	return bson.E{Key: key, Value: bson.D{{Key: mongoOperator, Value: value}}}, decoded.Direction, sortOrder, nil
}

// position is a debt's cursor value on the listing key.
func position(key string, debt *model.FeeDebt) string {
	if key == "seq" {
		return strconv.FormatInt(debt.Seq, 10)
	}

	return debt.ID
}

// applyChange seeds the debt, then records the change unless its entry is already
// there. The seed filters on _id alone, so MongoDB retries a concurrent seed's
// duplicate-key insert as a no-op update instead of failing it.
func applyChange(ctx context.Context, coll *mongo.Collection, id string, seed, filter, update bson.D) error {
	if _, err := coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, seed, options.UpdateOne().SetUpsert(true)); err != nil {
		return err
	}

	_, err := coll.UpdateOne(ctx, filter, update)

	return err
}

// changeUpdate builds the seed that creates a debt from whichever of its changes lands
// first, with remaining = opened, and the update guarded on the change's entry key that
// moves remaining by -settled -canceled +reopened. Opened and refunded leave it alone.
func changeUpdate(record command.FeeDebtRecord, change accounting.FeeDebtChange) (bson.D, bson.D, bson.D, error) {
	delta := decimal.Zero

	switch change.Kind {
	case accounting.FeeDebtOpened, accounting.FeeDebtRefunded:
	case accounting.FeeDebtReopened:
		delta = change.Amount
	case accounting.FeeDebtSettled, accounting.FeeDebtCanceled:
		delta = change.Amount.Neg()
	default:
		return nil, nil, nil, fmt.Errorf("unknown fee debt change kind %q", change.Kind)
	}

	increment, err := bson.ParseDecimal128(delta.String())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fee debt change amount: %w", err)
	}

	opened, err := bson.ParseDecimal128(change.Opened.String())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fee debt opened amount: %w", err)
	}

	seed := bson.D{{Key: "$setOnInsert", Value: bson.D{
		{Key: "organization_id", Value: record.OrganizationID.String()},
		{Key: "ledger_id", Value: record.LedgerID.String()},
		{Key: "debtor_balance_ref", Value: change.DebtorRef},
		{Key: "credit_balance_ref", Value: change.CreditRef},
		{Key: "origin_transaction_id", Value: change.OriginTransactionID.String()},
		{Key: "asset_code", Value: change.AssetCode},
		{Key: "seq", Value: change.Seq},
		{Key: "opened_amount", Value: opened},
		{Key: "remaining", Value: opened},
		{Key: "entries", Value: bson.A{}},
		{Key: "created_at", Value: record.AppliedAt},
		{Key: "updated_at", Value: record.AppliedAt},
	}}}

	key := change.TransactionID.String() + ":" + change.PostingRef + ":" + string(change.Kind)
	entry := model.FeeDebtEntry{
		Key:           key,
		Kind:          string(change.Kind),
		TransactionID: change.TransactionID.String(),
		PostingRef:    change.PostingRef,
		Amount:        change.Amount.String(),
		AppliedAt:     record.AppliedAt,
	}

	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: "remaining", Value: increment}}},
		{Key: "$push", Value: bson.D{{Key: "entries", Value: bson.D{
			{Key: "$each", Value: bson.A{entry}},
			{Key: "$sort", Value: bson.D{{Key: "applied_at", Value: 1}, {Key: "key", Value: 1}}},
		}}}},
		{Key: "$min", Value: bson.D{{Key: "created_at", Value: record.AppliedAt}}},
		{Key: "$max", Value: bson.D{{Key: "updated_at", Value: record.AppliedAt}}},
	}

	if change.Kind == accounting.FeeDebtOpened {
		set := bson.D{{Key: "opened_at", Value: record.AppliedAt}}
		if record.FeePackageID != "" {
			set = append(set, bson.E{Key: "fee_package_id", Value: record.FeePackageID})
		}

		update = append(update, bson.E{Key: "$set", Value: set})
	}

	filter := bson.D{
		{Key: "_id", Value: change.DebtID},
		{Key: "entries.key", Value: bson.D{{Key: "$ne", Value: key}}},
	}

	return seed, filter, update, nil
}
