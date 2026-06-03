// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package support

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// ScenarioContext holds shared state between step definitions within a single scenario.
// It accumulates context from Given/And steps and is consumed by When/Then steps.
// Each Godog scenario gets a fresh ScenarioContext instance.
type ScenarioContext struct {
	// --- Rule state ---
	// Map of rule name (normalized) → rule ID for cross-step references.
	Rules map[string]string

	// Last rule operation result (for Then assertions).
	LastRule     RuleResponse
	LastRuleHTTP int

	// --- Limit state ---
	// Map of limit name → limit ID for cross-step references.
	Limits map[string]string

	// Last limit operation result.
	LastLimit     LimitResponse
	LastLimitHTTP int

	// Last usage check result.
	LastUsage     UsageSnapshot
	LastUsageHTTP int

	// --- Validation state ---
	// Last validation response.
	LastValidation     ValidationResponse
	LastValidationHTTP int

	// Validation history for multi-transaction scenarios.
	ValidationHistory  []ValidationResponse
	LastValidationList ListValidationsResponse

	// --- Audit state ---
	LastAuditEvents     ListAuditEventsResponse
	LastAuditEventsHTTP int

	LastHashVerification     HashChainVerification
	LastHashVerificationHTTP int

	// --- Builders (context accumulation for decomposed steps) ---
	PendingRule        *PendingRule
	PendingLimit       *PendingLimit
	PendingTransaction *PendingTransaction

	// --- Cleanup ---
	// RuleIDs and LimitIDs to clean up after the scenario.
	RuleCleanup  []string
	LimitCleanup []string

	// --- Merchant UUID mapping (for J8) ---
	MerchantUUIDs map[string]string

	// --- Filter state (for multi-step filter→assert patterns) ---
	LastFilteredRuleID string

	// --- Audit atomicity state ---
	// When set, lifecycle steps (activates the rule/limit) tolerate 5xx
	// responses instead of failing fast, and capture status+body for
	// downstream assertions.
	ExpectActivationFailure bool
	LastActivationHTTP      int
	LastActivationBody      []byte

	// DB handle and fault-injection cleanup callbacks. The DB handle is
	// opened lazily via DB() and closed automatically by the Cleanup()
	// method. Fault triggers are registered here so they are dropped
	// after the scenario even when the scenario aborts early.
	db            *sql.DB
	faultCleanups []func()
}

// PendingRule accumulates fields for a rule being built via Given/And steps.
type PendingRule struct {
	Name       string
	Expression string
	Action     string
	Scopes     []testutil.ScopeInput
}

// PendingLimit accumulates fields for a limit being built via Given/And steps.
type PendingLimit struct {
	Name      string
	LimitType string
	MaxAmount decimal.Decimal
	Currency  string
	Scopes    []testutil.ScopeInput
}

// PendingTransaction accumulates fields for a validation request being built via Given/And steps.
type PendingTransaction struct {
	TransactionType string
	SubType         string
	Amount          decimal.Decimal
	Currency        string
	AccountID       string
	SegmentID       string
	Metadata        map[string]any
	Merchant        *testutil.MerchantContext
}

// NewScenarioContext creates a fresh ScenarioContext for a new scenario.
func NewScenarioContext() *ScenarioContext {
	return &ScenarioContext{
		Rules:         make(map[string]string),
		Limits:        make(map[string]string),
		MerchantUUIDs: make(map[string]string),
	}
}

// RegisterRule stores a rule name → ID mapping and registers it for cleanup.
// The name is stored as-is (the API normalizes it: lowercase + collapse whitespace).
func (sc *ScenarioContext) RegisterRule(name, id string) {
	sc.Rules[name] = id
	sc.RuleCleanup = append(sc.RuleCleanup, id)
}

// FindRuleID looks up a rule ID by name, trying local context first, then API.
func (sc *ScenarioContext) FindRuleID(name string) string {
	// Try exact match first (API-normalized names are stored as returned)
	if id, ok := sc.Rules[name]; ok {
		return id
	}

	// Try lowercase match (rule names are normalized by the API)
	lower := strings.ToLower(strings.TrimSpace(name))
	if id, ok := sc.Rules[lower]; ok {
		return id
	}

	// Try case-insensitive search across all stored names
	for storedName, id := range sc.Rules {
		if strings.EqualFold(storedName, name) {
			return id
		}
	}

	// Fallback: search via API (for cross-scenario references).
	// Only cache locally — do NOT register for cleanup since this scenario did not create it.
	rule, err := FindRuleByNameE(name)
	if err == nil && rule.ID != "" {
		sc.Rules[rule.Name] = rule.ID

		return rule.ID
	}

	return ""
}

// FindLimitID looks up a limit ID by name, trying local context first, then API.
func (sc *ScenarioContext) FindLimitID(name string) string {
	// Try exact match first (API-normalized names are stored as returned)
	if id, ok := sc.Limits[name]; ok {
		return id
	}

	// Try lowercase match (limit names may be normalized by the API)
	lower := strings.ToLower(strings.TrimSpace(name))
	if id, ok := sc.Limits[lower]; ok {
		return id
	}

	// Try case-insensitive search across all stored names
	for storedName, id := range sc.Limits {
		if strings.EqualFold(storedName, name) {
			return id
		}
	}

	// Fallback: search via API (for cross-scenario references).
	// Only cache locally — do NOT register for cleanup since this scenario did not create it.
	limit, err := FindLimitByNameE(name)
	if err == nil && limit.ID != "" {
		sc.Limits[limit.Name] = limit.ID

		return limit.ID
	}

	return ""
}

// RegisterLimit stores a limit name → ID mapping and registers it for cleanup.
func (sc *ScenarioContext) RegisterLimit(name, id string) {
	sc.Limits[name] = id
	sc.LimitCleanup = append(sc.LimitCleanup, id)
}

// Cleanup deletes all created rules and limits (best-effort).
func (sc *ScenarioContext) Cleanup() {
	// Drop fault-injection triggers FIRST so that any rule/limit deletes
	// issued next do not re-trigger the injected audit failure.
	for i := len(sc.faultCleanups) - 1; i >= 0; i-- {
		sc.faultCleanups[i]()
	}

	sc.faultCleanups = nil

	for _, id := range sc.RuleCleanup {
		if err := CleanupRuleE(id); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup rule %s: %v\n", id, err)
		}
	}

	for _, id := range sc.LimitCleanup {
		if err := CleanupLimitE(id); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup limit %s: %v\n", id, err)
		}
	}

	if sc.db != nil {
		if err := sc.db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close scenario db: %v\n", err)
		}

		sc.db = nil
	}
}

// ScenarioTearDown runs the minimum teardown required between scenarios:
// drop any fault-injection triggers this scenario installed and close the
// per-scenario DB handle. It does NOT delete rules or limits — those are
// preserved across scenarios by design (sequential-journey feature files).
func (sc *ScenarioContext) ScenarioTearDown() {
	// Drop triggers in reverse installation order.
	for i := len(sc.faultCleanups) - 1; i >= 0; i-- {
		sc.faultCleanups[i]()
	}

	sc.faultCleanups = nil

	// Reset atomicity-flow state so a latent flag cannot leak into the next
	// scenario. Today InitializeScenario allocates a fresh ScenarioContext
	// per scenario, so a leak is impossible; resetting defensively here
	// keeps the teardown correct if that wiring is ever replaced with a
	// persistent context (e.g. a pooled/reused fixture).
	sc.ExpectActivationFailure = false
	sc.LastActivationHTTP = 0
	sc.LastActivationBody = nil

	if sc.db != nil {
		if err := sc.db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close scenario db: %v\n", err)
		}

		sc.db = nil
	}
}

// DB returns the lazily-opened PostgreSQL connection scoped to this
// scenario. The connection is closed automatically by Cleanup().
func (sc *ScenarioContext) DB() (*sql.DB, error) {
	if sc.db != nil {
		return sc.db, nil
	}

	db, err := OpenTestDB()
	if err != nil {
		return nil, err
	}

	sc.db = db

	return db, nil
}

// AddFaultCleanup registers a cleanup callback for a fault-injection
// trigger installed during the scenario. Nil callbacks are ignored so
// ScenarioTearDown never panics on an accidental nil registration.
func (sc *ScenarioContext) AddFaultCleanup(fn func()) {
	if fn == nil {
		return
	}

	sc.faultCleanups = append(sc.faultCleanups, fn)
}

// InitPendingRule starts a new rule builder.
func (sc *ScenarioContext) InitPendingRule(name, action string) {
	sc.PendingRule = &PendingRule{
		Name:   name,
		Action: action,
	}
}

// InitPendingLimit starts a new limit builder.
func (sc *ScenarioContext) InitPendingLimit(name, limitType string) {
	sc.PendingLimit = &PendingLimit{
		Name:      name,
		LimitType: limitType,
		Currency:  "BRL", // Default currency
	}
}

// InitPendingTransaction starts a new transaction builder.
func (sc *ScenarioContext) InitPendingTransaction(txType string, amount decimal.Decimal) {
	sc.PendingTransaction = &PendingTransaction{
		TransactionType: txType,
		Amount:          amount,
		Currency:        "BRL",
		Metadata:        make(map[string]any),
	}
}

// BuildValidationRequest converts a PendingTransaction to a ValidationRequest.
func (sc *ScenarioContext) BuildValidationRequest() *testutil.ValidationRequest {
	pt := sc.PendingTransaction
	if pt == nil {
		return nil
	}

	req := &testutil.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(NextRequestID()).String(),
		TransactionType: pt.TransactionType,
		SubType:         pt.SubType,
		Amount:          pt.Amount,
		Currency:        pt.Currency,
	}

	if pt.AccountID != "" {
		req.Account = &testutil.AccountContext{ID: pt.AccountID}
	}

	if pt.SegmentID != "" {
		req.Segment = &testutil.SegmentContext{ID: pt.SegmentID}
	}

	if pt.Merchant != nil {
		req.Merchant = pt.Merchant
	}

	if len(pt.Metadata) > 0 {
		req.Metadata = pt.Metadata
	}

	return req
}

// requestIDCounter provides unique request IDs for validations within a test run.
var requestIDCounter int64 = 50000

// NextRequestID returns the next available request ID base.
func NextRequestID() int64 {
	return atomic.AddInt64(&requestIDCounter, 1)
}
