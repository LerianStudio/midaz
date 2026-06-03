// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// isNonRetryableHandlerError classifies whether a handler error is permanent
// (non-retryable) and the message should be dropped, or transient and the
// message should be redelivered.
//
// This function acts as a DEFENSIVE RETRY GUARD at the handler level. The
// lib-commons multi-tenant consumer calls msg.Nack(false, true) for any
// non-nil error returned by the handler, causing infinite redelivery loops
// for permanent errors. By returning nil for permanent errors (after logging),
// the handler tells lib-commons to Ack the message instead.
//
// Classification:
//   - context.Canceled / context.DeadlineExceeded → non-retryable
//   - Permanent tenant errors (not found, suspended, closed, not configured) → non-retryable
//   - Domain validation/business errors (pkg.ValidationError, etc.) → non-retryable
//   - TPL-* error codes in the error string → non-retryable
//   - Everything else (network, DB, circuit breaker open) → retryable
func isNonRetryableHandlerError(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation / deadline exceeded: no point retrying.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// JSON parse errors — malformed messages will never succeed on retry.
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return true
	}

	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return true
	}

	// Permanent tenant-manager errors.
	if isPermanentTenantError(err) {
		return true
	}

	// Domain-level business/validation errors that will never succeed on retry.
	if isNonRetryableDomainError(err) {
		return true
	}

	// TPL-XXXX error codes embedded in the error message.
	if strings.Contains(err.Error(), "TPL-") {
		return true
	}

	// Last-resort heuristic: catch permanent errors not yet wrapped in typed errors.
	if isPermanentErrorByPattern(err.Error()) {
		return true
	}

	return false
}

// isPermanentTenantError classifies tenant-manager errors as permanent.
// Permanent errors (tenant not found, suspended, service not configured,
// manager closed) will never succeed on retry. Transient errors (circuit
// breaker open, network issues) may resolve on subsequent attempts.
//
// NOTE: This function already exists at consumer.go:resolveMultiTenantMongo
// for observability logging. Here it is used for retry classification.
func isPermanentTenantError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return true
	}

	if errors.Is(err, tmcore.ErrServiceNotConfigured) {
		return true
	}

	if errors.Is(err, tmcore.ErrManagerClosed) {
		return true
	}

	if tmcore.IsTenantSuspendedError(err) {
		return true
	}

	return false
}

// isNonRetryableDomainError checks for known non-retryable error types
// from the reporter application domain.
//
// NOTE: Duplicated from pkg/rabbitmq/error_classifier.go to avoid import
// cycles (bootstrap → pkg/rabbitmq would create a dependency loop).
func isNonRetryableDomainError(err error) bool {
	if _, ok := errors.AsType[pkg.ValidationError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.EntityNotFoundError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.ValidationKnownFieldsError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.ValidationUnknownFieldsError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.UnprocessableOperationError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.EntityConflictError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.ForbiddenError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.UnauthorizedError](err); ok {
		return true
	}

	if _, ok := errors.AsType[pkg.FailedPreconditionError](err); ok {
		return true
	}

	// SchemaAmbiguityError — permanent data source configuration error
	// (ambiguous table reference across multiple schemas).
	_, ok := errors.AsType[*pkg.SchemaAmbiguityError](err)

	return ok
}

// isPermanentErrorByPattern is a last-resort safety net that catches permanent
// errors not yet wrapped in typed domain errors. It uses string matching on the
// error message — prefer wrapping errors at the source with typed errors instead.
//
// Patterns are intentionally specific to avoid false positives from transient
// errors that happen to contain common substrings. Each pattern targets a known
// permanent error message from the report generation pipeline.
func isPermanentErrorByPattern(errMsg string) bool {
	permanentPatterns := []string{
		"key not configured",                     // crypto config missing
		"client is not configured",               // storage client not injected
		"data source not found",                  // datasource not registered in worker
		"has no tables",                          // empty mappedFields entry
		"does not support crm queries",           // CRM type assertion failure
		"no collections found matching prefix",   // CRM collection prefix has no matches
		"unsupported database type",              // unknown datasource type
		"unexpected schema result type",          // circuit breaker returned wrong type
		"unexpected query result type",           // circuit breaker returned wrong type
		"is unavailable (initialization failed)", // datasource permanently failed init
		"failed to initialize cipher",            // crypto key invalid
		"encrypted data is empty",                // fetcher returned empty payload
		"ciphertext too short",                   // corrupted encrypted data
	}

	lower := strings.ToLower(errMsg)

	for _, pattern := range permanentPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}
