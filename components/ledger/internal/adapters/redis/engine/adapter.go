// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/valkey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/balance_engine.lua
var accountingScriptSource string

var accountingScript = redis.NewScript(accountingScriptSource)

// RedisClientProvider resolves the authenticated tenant's connection configuration.
type RedisClientProvider interface {
	GetClient(context.Context) (redis.UniversalClient, error)
}

// Adapter executes the accounting protocol without enabling any application path.
// It currently supports standalone Redis clients; cluster and ring transports
// require a separately verified no-retransmission implementation.
type Adapter struct {
	provider RedisClientProvider
	limits   Limits
}

// TechnicalError distinguishes confirmed pre-write failures from uncertain outcomes.
// Underlying errors remain available through errors.Is and errors.As.
type TechnicalError struct {
	Code          string
	Indeterminate bool
	Err           error
}

func (e *TechnicalError) Error() string {
	if e.Err == nil {
		return "accounting execution " + e.Code
	}

	return "accounting execution " + e.Code + ": " + e.Err.Error()
}

func (e *TechnicalError) Unwrap() error {
	return e.Err
}

// EngineFailureCode exposes the stable technical classification without
// requiring callers to depend on this adapter's concrete error type.
func (e *TechnicalError) EngineFailureCode() string {
	if e == nil {
		return ""
	}

	return e.Code
}

// OutcomeIndeterminate reports whether execution may have committed despite
// returning an error.
func (e *TechnicalError) OutcomeIndeterminate() bool {
	return e != nil && e.Indeterminate
}

// NewAdapter requires explicit operational limits and does not alter a shared client.
func NewAdapter(provider RedisClientProvider, limits Limits) (*Adapter, error) {
	if provider == nil || (reflect.ValueOf(provider).Kind() == reflect.Pointer && reflect.ValueOf(provider).IsNil()) || limits.MaxPreparedBytes <= 0 || limits.MaxRequestBytes <= 0 || limits.MaxTransactions <= 0 || limits.MaxPostings <= 0 || limits.MaxBalances <= 0 || limits.MaxRecoveryBytes <= 0 {
		return nil, errors.New("accounting adapter requires a provider and positive limits")
	}

	return &Adapter{provider: provider, limits: limits}, nil
}

func technical(code string, uncertain bool, err error) error {
	return &TechnicalError{Code: code, Indeterminate: uncertain, Err: err}
}

// Execute uses a dedicated pool with retransmission disabled. A fresh pool per
// execution preserves tenant-specific auth/TLS options without mutating or
// closing the provider's shared pool. Only confirmed NOSCRIPT permits fallback.
func (a *Adapter) Execute(ctx context.Context, input command.EngineExecution) (*engine.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, technical("context_canceled", false, err)
	}

	resolved, err := resolveAdapterKeys(ctx, input.Request)
	if err != nil {
		return nil, technical("invalid_scope", false, err)
	}

	prepared, err := prepareExecution(ctx, input, a.limits, resolved)
	if err != nil {
		return nil, technical("invalid_request", false, err)
	}

	if err := command.ValidateBalanceEngineRecovery(input); err != nil {
		return nil, technical("invalid_recovery", false, err)
	}

	if err := validateRecoveryTenant(input, resolved.TenantID); err != nil {
		return nil, technical("invalid_recovery", false, err)
	}

	shared, err := a.provider.GetClient(ctx)
	if err != nil {
		return nil, technical("connection_unavailable", false, err)
	}

	source, ok := shared.(*redis.Client)
	if !ok || source == nil {
		return nil, technical("unsupported_transport", false, errors.New("accounting execution requires a standalone Redis client"))
	}

	options := *source.Options()
	options.MaxRetries = -1

	client := redis.NewClient(&options)
	defer client.Close()

	if err := ctx.Err(); err != nil {
		return nil, technical("context_canceled", false, err)
	}

	args := []any{prepared.Payload, a.limits.MaxRequestBytes, a.limits.MaxPreparedBytes}

	response, err := client.EvalSha(ctx, accountingScript.Hash(), prepared.Keys, args...).Result()
	if isNoScript(err) {
		response, err = client.Eval(ctx, accountingScriptSource, prepared.Keys, args...).Result()
	}

	if err != nil {
		return nil, classifyAccountingError(err, input.Request, prepared.Keys)
	}

	raw, ok := response.(string)
	if !ok || len(raw) > a.limits.MaxPreparedBytes {
		return nil, technical("invalid_response", true, errors.New("unexpected accounting response type or size"))
	}

	result, err := DecodeResult([]byte(raw), input.Request)
	if err != nil {
		return nil, technical("invalid_response", true, err)
	}

	return result, nil
}

func validateRecoveryTenant(input command.EngineExecution, tenantID string) error {
	for _, recovery := range input.Recovery {
		var scope struct {
			TenantID string `json:"tenantId"`
		}

		if err := json.Unmarshal(recovery.Payload, &scope); err != nil {
			return fmt.Errorf("decode accounting recovery scope: %w", err)
		}

		if scope.TenantID != tenantID {
			return fmt.Errorf("%w: recovery tenant differs from authenticated scope", command.ErrInvalidBalanceEngineRecovery)
		}
	}

	return nil
}

func isNoScript(err error) bool {
	var reply redis.Error
	return errors.As(err, &reply) && strings.HasPrefix(reply.Error(), "NOSCRIPT ")
}

func resolveAdapterKeys(ctx context.Context, request engine.Request) (resolvedExecutionKeys, error) {
	scope := request.OrganizationID.String() + ":" + request.LedgerID.String()

	resolved := resolvedExecutionKeys{
		TenantID: tmcore.GetTenantIDContext(ctx), Schedule: utils.BalanceSyncScheduleKey,
		Recovery: "backup_queue:{transactions}",
		Receipts: "engine:{transactions}:receipts:" + scope,
		Guards:   "engine:{transactions}:guards:" + scope,
		Balances: make(map[string]resolvedBalanceKeys, len(request.Balances)),
	}
	for _, key := range []*string{&resolved.Schedule, &resolved.Recovery, &resolved.Receipts, &resolved.Guards} {
		prefixed, err := tmvalkey.GetKeyContext(ctx, *key)
		if err != nil {
			return resolvedExecutionKeys{}, err
		}

		*key = prefixed
	}

	for _, balance := range request.Balances {
		key := utils.BalanceInternalKey(request.OrganizationID, request.LedgerID, balance.BalanceRef)

		prefixed, err := tmvalkey.GetKeyContext(ctx, key)
		if err != nil {
			return resolvedExecutionKeys{}, err
		}

		resolved.Balances[balance.BalanceRef] = resolvedBalanceKeys{Balance: prefixed, Deleted: prefixed + ":deleted"}
	}

	return resolved, nil
}

func classifyAccountingError(err error, request engine.Request, keys []string) error {
	var server redis.Error
	if !errors.As(err, &server) {
		return technical("transport", true, err)
	}

	reply := strings.TrimPrefix(server.Error(), "ERR ")
	if payload, ok := strings.CutPrefix(reply, "MIDAZ_ENGINE_V1 "); ok {
		var failure engine.Failure
		if decodeStrict([]byte(payload), &failure) != nil || validateFailure(failure, request) != nil {
			return technical("invalid_failure", true, err)
		}

		return &failure
	}

	if payload, ok := strings.CutPrefix(reply, "MIDAZ_ENGINE_TECH_V1 "); ok {
		var failure struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if decodeStrict([]byte(payload), &failure) != nil {
			return technical("invalid_technical_failure", true, err)
		}

		switch failure.Code {
		case "invalid_json", "invalid_protocol", "invalid_balance", "balance_identity_mismatch", "wrong_key_type", "execution_fingerprint_conflict", "execution_guard_conflict", "version_overflow", "invalid_companion", "prepared_bytes_exceeded", "request_bytes_exceeded", "serialization_failed", "script_runtime_failed":
			return technical(failure.Code, false, err)
		case "indeterminate", "execution_outcome_unknown", "invalid_receipt":
			return technical(failure.Code, true, err)
		default:
			return technical("unknown_technical_failure", true, err)
		}
	}

	if payload, ok := strings.CutPrefix(reply, "BALANCE_LIMIT_NORMALIZATION_REQUIRED:"); ok {
		var reported []string
		if json.Unmarshal([]byte(payload), &reported) != nil || len(reported) == 0 {
			return technical("invalid_normalization_failure", true, err)
		}

		allowed := make(map[string]bool, len(keys)/2)
		for i := 4; i < len(keys); i += 2 {
			allowed[keys[i]] = true
		}

		for _, key := range reported {
			if !allowed[key] {
				return technical("invalid_normalization_failure", true, err)
			}
		}

		return technical("normalization_required", false, err)
	}

	return technical("script_runtime", true, err)
}

func validateFailure(failure engine.Failure, request engine.Request) error {
	switch failure.Code {
	case engine.FailureInsufficientFunds, engine.FailureOverdraftLimitExceeded, engine.FailureOverdraftNotEligible, engine.FailureOverdraftCompanionMissing, engine.FailureStaleVersion, engine.FailureBalanceDeleted, engine.FailureOnHoldUnderflow, engine.FailureBalanceMissing:
	default:
		return errors.New("unknown accounting refusal code")
	}

	if failure.TransactionIndex < 0 || failure.TransactionIndex >= len(request.Transactions) {
		return errors.New("invalid accounting refusal transaction")
	}

	postings := request.Transactions[failure.TransactionIndex].Postings
	if failure.PostingIndex < 0 || failure.PostingIndex >= len(postings) || failure.BalanceRef == "" {
		return errors.New("invalid accounting refusal posting")
	}

	posting := postings[failure.PostingIndex]
	if failure.BalanceRef == posting.BalanceRef {
		return nil
	}

	var origin *engine.BalanceSnapshot

	for i := range request.Balances {
		if request.Balances[i].BalanceRef == posting.BalanceRef {
			origin = &request.Balances[i]
			break
		}
	}

	for _, balance := range request.Balances {
		if origin != nil && balance.BalanceRef == failure.BalanceRef && balance.AccountID == origin.AccountID && balance.Key == "overdraft" {
			return nil
		}
	}

	return errors.New("uncorrelated accounting refusal balance")
}

type resultEnvelope struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Movements       []json.RawMessage `json:"movements"`
	Final           []json.RawMessage `json:"final"`
}

type resultState struct {
	Available     string `json:"available"`
	OnHold        string `json:"onHold"`
	OverdraftUsed string `json:"overdraftUsed"`
	Version       string `json:"version"`
}

type resultMovement struct {
	Ref            string             `json:"ref"`
	TransactionID  string             `json:"transactionId"`
	PostingRef     string             `json:"postingRef"`
	Role           string             `json:"role"`
	BalanceRef     string             `json:"balanceRef"`
	Type           engine.PostingType `json:"type"`
	Amount         string             `json:"amount"`
	OverdraftDelta string             `json:"overdraftDelta"`
	Before         json.RawMessage    `json:"before"`
	After          json.RawMessage    `json:"after"`
}

type resultBalance struct {
	BalanceRef string `json:"balanceRef"`
	wireBalanceSnapshot
}

// DecodeResult validates the protocol and origin correlation without coercing
// monetary JSON numbers, object-shaped arrays or rounded versions.
func DecodeResult(raw []byte, request engine.Request) (*engine.Result, error) {
	var response resultEnvelope
	if err := decodeStrict(raw, &response); err != nil {
		return nil, err
	}

	if response.ProtocolVersion != 1 || response.Movements == nil || response.Final == nil {
		return nil, errors.New("invalid accounting result envelope")
	}

	result := &engine.Result{Movements: make([]engine.Movement, 0, len(response.Movements)), Final: make([]engine.BalanceSnapshot, 0, len(response.Final))}
	last := make(map[string]engine.BalanceState)
	firstTouch := make([]string, 0, len(response.Final))
	previousOrdinal := -1

	for _, rawMovement := range response.Movements {
		movement, ordinal, err := decodeMovement(rawMovement, request)
		if err != nil {
			return nil, err
		}

		if ordinal <= previousOrdinal {
			return nil, errors.New("unordered or repeated accounting movement")
		}

		if err := validateCompanionSequence(movement, result.Movements); err != nil {
			return nil, err
		}

		previousOrdinal = ordinal

		if before, exists := last[movement.BalanceRef]; exists {
			if !sameState(before, movement.Before) {
				return nil, errors.New("discontinuous accounting movement state")
			}
		} else {
			firstTouch = append(firstTouch, movement.BalanceRef)
		}

		last[movement.BalanceRef] = movement.After
		result.Movements = append(result.Movements, movement)
	}

	if len(result.Movements) > 0 && requiresCompanion(result.Movements[len(result.Movements)-1]) {
		return nil, errors.New("primary debt change has no companion")
	}

	if len(response.Final) != len(firstTouch) {
		return nil, errors.New("accounting final set differs from touched balances")
	}

	for i, rawBalance := range response.Final {
		balance, err := decodeFinalBalance(rawBalance, request)
		if err != nil {
			return nil, err
		}

		if balance.BalanceRef != firstTouch[i] || !sameState(last[balance.BalanceRef], engine.BalanceState{Available: balance.Available, OnHold: balance.OnHold, OverdraftUsed: balance.OverdraftUsed, Version: balance.Version}) {
			return nil, errors.New("accounting final state does not match last movement")
		}

		result.Final = append(result.Final, balance)
	}

	return result, nil
}

func validateCompanionSequence(movement engine.Movement, previous []engine.Movement) error {
	if movement.Role != engine.RoleOverdraftCompanion {
		if len(previous) > 0 && requiresCompanion(previous[len(previous)-1]) {
			return errors.New("primary debt change has no companion")
		}

		return nil
	}

	if len(previous) == 0 {
		return errors.New("companion movement has no primary")
	}

	primary := previous[len(previous)-1]
	if primary.Role != engine.RolePrimary || primary.TransactionID != movement.TransactionID || primary.PostingRef != movement.PostingRef || primary.OverdraftDelta.IsZero() || !movement.Amount.Equal(primary.OverdraftDelta.Abs()) {
		return errors.New("companion movement does not match primary debt change")
	}

	if (primary.OverdraftDelta.IsPositive() && movement.Type != engine.PostingDebit) || (primary.OverdraftDelta.IsNegative() && movement.Type != engine.PostingCredit) {
		return errors.New("companion movement has incorrect repayment direction")
	}

	return nil
}

func requiresCompanion(movement engine.Movement) bool {
	return movement.Role == engine.RolePrimary && !movement.OverdraftDelta.IsZero()
}

func decodeMovement(raw []byte, request engine.Request) (engine.Movement, int, error) {
	var wire resultMovement
	if err := decodeStrict(raw, &wire); err != nil {
		return engine.Movement{}, 0, err
	}

	transactionID, err := uuid.Parse(wire.TransactionID)
	if err != nil {
		return engine.Movement{}, 0, errors.New("invalid movement transaction ID")
	}

	posting, balance, ordinal, err := correlateMovement(wire, transactionID, request)
	if err != nil {
		return engine.Movement{}, 0, err
	}

	expectedRef := transactionID.String() + ":" + strconv.Itoa(len(posting.Ref)) + ":" + posting.Ref + ":" + wire.Role + ":0"
	if wire.Ref != expectedRef || !validPostingType(wire.Type) {
		return engine.Movement{}, 0, errors.New("invalid movement identity or type")
	}

	before, err := decodeState(wire.Before)
	if err != nil {
		return engine.Movement{}, 0, err
	}

	after, err := decodeState(wire.After)
	if err != nil {
		return engine.Movement{}, 0, err
	}

	amount, err := strictDecimal(wire.Amount)
	if err != nil {
		return engine.Movement{}, 0, errors.New("invalid movement amount")
	}

	delta, err := strictDecimal(wire.OverdraftDelta)
	if err != nil {
		return engine.Movement{}, 0, errors.New("invalid movement overdraft delta")
	}

	movement := engine.Movement{Ref: wire.Ref, TransactionID: transactionID, PostingRef: wire.PostingRef, Role: wire.Role, BalanceRef: wire.BalanceRef, Type: wire.Type, Amount: amount, OverdraftDelta: delta, Before: before, After: after}
	if err := validateMovementTransition(movement, posting, balance); err != nil {
		return engine.Movement{}, 0, err
	}

	return movement, ordinal, nil
}

func validateMovementTransition(movement engine.Movement, posting engine.Posting, balance engine.BalanceSnapshot) error {
	before, after := movement.Before, movement.After
	if movement.Amount.IsNegative() || movement.Amount.GreaterThan(posting.Amount) {
		return errors.New("invalid movement amount")
	}

	if before.Version == int64(^uint64(0)>>1) || after.Version != before.Version+1 || sameMoney(before, after) {
		return errors.New("invalid movement state transition")
	}

	if balance.AccountType != "external" && (before.Available.IsNegative() || after.Available.IsNegative()) {
		return errors.New("invalid movement available balance")
	}

	if movement.Role == engine.RolePrimary && (!movement.OverdraftDelta.Equal(after.OverdraftUsed.Sub(before.OverdraftUsed)) || movement.Type != posting.Type) {
		return errors.New("invalid primary movement correlation")
	}

	if movement.Role == engine.RoleOverdraftCompanion && (!movement.OverdraftDelta.IsZero() || (movement.Type != engine.PostingDebit && movement.Type != engine.PostingCredit)) {
		return errors.New("invalid companion movement correlation")
	}

	return nil
}

func correlateMovement(wire resultMovement, transactionID uuid.UUID, request engine.Request) (engine.Posting, engine.BalanceSnapshot, int, error) {
	posting, ordinal, found := findPosting(request, transactionID, wire.PostingRef)
	if !found {
		return engine.Posting{}, engine.BalanceSnapshot{}, 0, errors.New("unknown movement origin")
	}

	source, sourceExists := findBalance(request, posting.BalanceRef)

	target, targetExists := findBalance(request, wire.BalanceRef)
	if sourceExists && targetExists {
		if wire.Role == engine.RolePrimary && source.BalanceRef == target.BalanceRef {
			return posting, target, ordinal, nil
		}

		if wire.Role == engine.RoleOverdraftCompanion && target.Key == "overdraft" && target.AccountID == source.AccountID && target.BalanceRef != source.BalanceRef {
			return posting, target, ordinal + 1, nil
		}
	}

	return engine.Posting{}, engine.BalanceSnapshot{}, 0, errors.New("uncorrelated movement balance or role")
}

func findPosting(request engine.Request, transactionID uuid.UUID, postingRef string) (engine.Posting, int, bool) {
	ordinal := 0

	for _, transaction := range request.Transactions {
		for _, posting := range transaction.Postings {
			if transaction.ID == transactionID && posting.Ref == postingRef {
				return posting, ordinal, true
			}

			ordinal += 2
		}
	}

	return engine.Posting{}, 0, false
}

func findBalance(request engine.Request, ref string) (engine.BalanceSnapshot, bool) {
	for _, balance := range request.Balances {
		if balance.BalanceRef == ref {
			return balance, true
		}
	}

	return engine.BalanceSnapshot{}, false
}

func decodeFinalBalance(raw []byte, request engine.Request) (engine.BalanceSnapshot, error) {
	var wire resultBalance
	if err := decodeStrict(raw, &wire); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	var balance *engine.BalanceSnapshot

	for i := range request.Balances {
		if request.Balances[i].BalanceRef == wire.BalanceRef {
			snapshot := request.Balances[i]
			balance = &snapshot

			break
		}
	}

	if balance == nil || wire.ID != balance.ID.String() || wire.AccountID != balance.AccountID.String() || wire.Alias != balance.Alias || wire.Key != balance.Key || wire.AssetCode != balance.AssetCode || wire.AccountType != balance.AccountType {
		return engine.BalanceSnapshot{}, errors.New("invalid final balance identity")
	}

	var err error
	if balance.Available, err = strictDecimal(wire.Available); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if balance.OnHold, err = strictDecimal(wire.OnHold); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if balance.OverdraftUsed, err = strictDecimal(wire.OverdraftUsed); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if balance.OverdraftLimit, err = strictDecimal(wire.OverdraftLimit); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	if balance.Version, err = strictVersion(wire.Version); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	balance.Direction, balance.BalanceScope = wire.Direction, wire.BalanceScope
	balance.AllowSending, balance.AllowReceiving = wire.AllowSending, wire.AllowReceiving

	balance.AllowOverdraft, balance.OverdraftLimitEnabled = wire.AllowOverdraft, wire.OverdraftLimitEnabled
	if _, err := prepareSnapshot(*balance, len(raw)); err != nil {
		return engine.BalanceSnapshot{}, err
	}

	return *balance, nil
}

func decodeState(raw []byte) (engine.BalanceState, error) {
	var wire resultState
	if err := decodeStrict(raw, &wire); err != nil {
		return engine.BalanceState{}, err
	}

	available, availableErr := strictDecimal(wire.Available)
	onHold, onHoldErr := strictDecimal(wire.OnHold)
	used, usedErr := strictDecimal(wire.OverdraftUsed)

	version, versionErr := strictVersion(wire.Version)
	if availableErr != nil || onHoldErr != nil || usedErr != nil || versionErr != nil || onHold.IsNegative() || used.IsNegative() {
		return engine.BalanceState{}, errors.New("invalid accounting state")
	}

	return engine.BalanceState{Available: available, OnHold: onHold, OverdraftUsed: used, Version: version}, nil
}

func strictDecimal(value string) (decimal.Decimal, error) {
	if value == "" || strings.ContainsAny(value, "eE+") {
		return decimal.Zero, errors.New("noncanonical accounting decimal")
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil || parsed.String() != value {
		return decimal.Zero, errors.New("noncanonical accounting decimal")
	}

	return parsed, nil
}

func strictVersion(value string) (int64, error) {
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != value {
		return 0, errors.New("invalid accounting version")
	}

	return version, nil
}

func sameMoney(left, right engine.BalanceState) bool {
	return left.Available.Equal(right.Available) && left.OnHold.Equal(right.OnHold) && left.OverdraftUsed.Equal(right.OverdraftUsed)
}

func sameState(left, right engine.BalanceState) bool {
	return left.Version == right.Version && sameMoney(left, right)
}

func decodeStrict(raw []byte, target any) error {
	fields, err := responseFields(raw)
	if err != nil {
		return err
	}

	if err := requireResponseFields(fields, reflect.TypeOf(target).Elem()); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode accounting response: %w", err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing accounting response content")
	}

	return nil
}

func responseFields(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("accounting response must be an object")
	}

	fields := make(map[string]json.RawMessage)

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		name, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid accounting response field")
		}

		if _, duplicate := fields[name]; duplicate {
			return nil, errors.New("duplicate accounting response field")
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}

		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("null accounting response field")
		}

		fields[name] = value
	}

	if _, err := decoder.Token(); err != nil {
		return nil, err
	}

	return fields, nil
}

func requireResponseFields(fields map[string]json.RawMessage, target reflect.Type) error {
	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		if field.Anonymous {
			if err := requireResponseFields(fields, field.Type); err != nil {
				return err
			}

			continue
		}

		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			if _, exists := fields[name]; !exists {
				return errors.New("missing accounting response field")
			}
		}
	}

	return nil
}

var _ command.BalanceEngine = (*Adapter)(nil)
