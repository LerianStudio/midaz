// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/stretchr/testify/assert"
)

func TestIsNonRetryableHandlerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		// nil → retryable (false)
		{
			name: "nil error is retryable",
			err:  nil,
			want: false,
		},

		// Context errors → non-retryable
		{
			name: "context.Canceled is non-retryable",
			err:  context.Canceled,
			want: true,
		},
		{
			name: "context.DeadlineExceeded is non-retryable",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "wrapped context.Canceled is non-retryable",
			err:  fmt.Errorf("operation failed: %w", context.Canceled),
			want: true,
		},

		// JSON parse errors → non-retryable
		{
			name: "json.SyntaxError is non-retryable",
			err:  &json.SyntaxError{Offset: 1},
			want: true,
		},
		{
			name: "json.UnmarshalTypeError is non-retryable",
			err:  &json.UnmarshalTypeError{Value: "string", Type: nil},
			want: true,
		},
		{
			name: "wrapped json.SyntaxError is non-retryable",
			err:  fmt.Errorf("parse notification: %w", &json.SyntaxError{Offset: 5}),
			want: true,
		},

		// Permanent tenant-manager errors → non-retryable
		{
			name: "ErrTenantNotFound is non-retryable",
			err:  tmcore.ErrTenantNotFound,
			want: true,
		},
		{
			name: "ErrServiceNotConfigured is non-retryable",
			err:  tmcore.ErrServiceNotConfigured,
			want: true,
		},
		{
			name: "ErrManagerClosed is non-retryable",
			err:  tmcore.ErrManagerClosed,
			want: true,
		},
		{
			name: "TenantSuspendedError is non-retryable",
			err:  &tmcore.TenantSuspendedError{TenantID: "t-1", Status: "suspended"},
			want: true,
		},

		// Domain validation/business errors → non-retryable
		{
			name: "ValidationError is non-retryable",
			err:  pkg.ValidationError{Code: "V001", Message: "invalid input"},
			want: true,
		},
		{
			name: "EntityNotFoundError is non-retryable",
			err:  pkg.EntityNotFoundError{EntityType: "report"},
			want: true,
		},
		{
			name: "EntityConflictError is non-retryable",
			err:  pkg.EntityConflictError{EntityType: "template", Message: "duplicate"},
			want: true,
		},
		{
			name: "ForbiddenError is non-retryable",
			err:  pkg.ForbiddenError{Message: "access denied"},
			want: true,
		},
		{
			name: "UnauthorizedError is non-retryable",
			err:  pkg.UnauthorizedError{Message: "no credentials"},
			want: true,
		},
		{
			name: "UnprocessableOperationError is non-retryable",
			err:  pkg.UnprocessableOperationError{Message: "invalid operation"},
			want: true,
		},
		{
			name: "FailedPreconditionError is non-retryable",
			err:  pkg.FailedPreconditionError{Message: "precondition failed"},
			want: true,
		},
		{
			name: "ValidationKnownFieldsError is non-retryable",
			err:  pkg.ValidationKnownFieldsError{Code: "V002", Message: "known field error"},
			want: true,
		},
		{
			name: "ValidationUnknownFieldsError is non-retryable",
			err:  pkg.ValidationUnknownFieldsError{Code: "V003", Message: "unknown fields"},
			want: true,
		},

		// SchemaAmbiguityError → non-retryable (permanent config error)
		{
			name: "SchemaAmbiguityError is non-retryable",
			err:  &pkg.SchemaAmbiguityError{Database: "mydb", Table: "users", Schemas: []string{"public", "sales"}},
			want: true,
		},
		{
			name: "wrapped SchemaAmbiguityError is non-retryable",
			err:  fmt.Errorf("query: %w", &pkg.SchemaAmbiguityError{Database: "mydb", Table: "orders", Schemas: []string{"a", "b"}}),
			want: true,
		},

		// TPL-* error codes → non-retryable
		{
			name: "TPL error code is non-retryable",
			err:  errors.New("TPL-0042 - template rendering failed"),
			want: true,
		},
		{
			name: "wrapped TPL error is non-retryable",
			err:  fmt.Errorf("render: %w", errors.New("TPL-0022 - missing field")),
			want: true,
		},

		// Wrapped domain error → non-retryable
		{
			name: "wrapped ValidationError is non-retryable",
			err:  fmt.Errorf("handler: %w", pkg.ValidationError{Code: "V001", Message: "bad input"}),
			want: true,
		},

		// Wrapped source errors (REP-006x codes from error wrapping at source)
		{
			name: "ValidationError REP-0060 (data source not found) is non-retryable",
			err:  pkg.ValidationError{Code: "REP-0060", Title: "Data Source Not Found", Message: "data source not found: mydb"},
			want: true,
		},
		{
			name: "ValidationError REP-0062 (unsupported db type) is non-retryable",
			err:  pkg.ValidationError{Code: "REP-0062", Title: "Unsupported Database Type", Message: "unsupported database type: oracle for database: mydb"},
			want: true,
		},
		{
			name: "FailedPreconditionError REP-0061 (unavailable datasource) is non-retryable",
			err:  pkg.FailedPreconditionError{Code: "REP-0061", Title: "Data Source Unavailable", Message: "datasource mydb is unavailable (initialization failed)"},
			want: true,
		},
		{
			name: "FailedPreconditionError REP-0066 (crypto config) is non-retryable",
			err:  pkg.FailedPreconditionError{Code: "REP-0066", Title: "CRM Crypto Not Configured", Message: "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM not configured"},
			want: true,
		},
		{
			name: "FailedPreconditionError REP-0068 (cipher init) is non-retryable",
			err:  pkg.FailedPreconditionError{Code: "REP-0068", Title: "Cipher Initialization Failed", Message: "failed to initialize cipher: invalid key"},
			want: true,
		},
		{
			name: "wrapped FailedPreconditionError REP-0066 is non-retryable",
			err:  fmt.Errorf("query: %w", pkg.FailedPreconditionError{Code: "REP-0066", Title: "CRM Crypto Not Configured", Message: "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM not configured"}),
			want: true,
		},
		// Heuristic pattern fallback (safety net for untyped errors)
		{
			name: "pattern: 'key not configured' is non-retryable",
			err:  fmt.Errorf("CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM key not configured"),
			want: true,
		},
		{
			name: "pattern: 'data source not found' is non-retryable",
			err:  fmt.Errorf("data source not found: legacy_db"),
			want: true,
		},
		{
			name: "pattern: 'failed to initialize cipher' is non-retryable",
			err:  fmt.Errorf("failed to initialize cipher: bad key"),
			want: true,
		},
		{
			name: "pattern: 'unsupported database type' is non-retryable",
			err:  fmt.Errorf("unsupported database type: oracle for database: mydb"),
			want: true,
		},
		{
			name: "pattern: 'encrypted data is empty' is non-retryable",
			err:  fmt.Errorf("encrypted data is empty"),
			want: true,
		},
		{
			name: "pattern: 'ciphertext too short' is non-retryable",
			err:  fmt.Errorf("ciphertext too short: expected 12 bytes, got 3"),
			want: true,
		},
		{
			name: "pattern: 'is unavailable (initialization failed)' is non-retryable",
			err:  fmt.Errorf("datasource mydb is unavailable (initialization failed)"),
			want: true,
		},
		// Negative heuristic cases — transient errors must NOT match
		{
			name: "pattern negative: 'service unavailable' without init context is retryable",
			err:  fmt.Errorf("service temporarily unavailable"),
			want: false,
		},
		{
			name: "pattern negative: generic 'not found' is retryable",
			err:  fmt.Errorf("document not found in collection"),
			want: false,
		},
		{
			name: "pattern negative: 'required' without specific field is retryable",
			err:  fmt.Errorf("additional authentication required"),
			want: false,
		},
		// Transient errors → retryable (false)
		{
			name: "connection refused is retryable",
			err:  errors.New("connection refused"),
			want: false,
		},
		{
			name: "ErrCircuitBreakerOpen is retryable",
			err:  tmcore.ErrCircuitBreakerOpen,
			want: false,
		},
		{
			name: "wrapped network error is retryable",
			err:  fmt.Errorf("publish: %w", errors.New("broken pipe")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isNonRetryableHandlerError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNotificationHandler_PermanentErrorsAreClassifiedNonRetryable(t *testing.T) {
	t.Parallel()

	// Consumer 2 (notification handler) specific error scenarios:
	// When ProcessFetcherNotification returns these errors, they must be
	// classified as non-retryable so the message is dropped instead of
	// causing an infinite redelivery loop.
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "EntityNotFoundError from ProcessFetcherNotification",
			err:  pkg.EntityNotFoundError{EntityType: "extraction_mapping", Message: "mapping not found for job"},
			want: true,
		},
		{
			name: "wrapped EntityNotFoundError from notification handler",
			err:  fmt.Errorf("process notification: %w", pkg.EntityNotFoundError{EntityType: "report", Message: "report not found"}),
			want: true,
		},
		{
			name: "ValidationError from notification payload parsing",
			err:  pkg.ValidationError{Code: "TPL-0050", Message: "invalid notification payload"},
			want: true,
		},
		{
			name: "UnprocessableOperationError from stale extraction",
			err:  pkg.UnprocessableOperationError{Message: "extraction already completed"},
			want: true,
		},
		{
			name: "transient DB error from notification handler is retryable",
			err:  errors.New("mongo: connection reset by peer"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isNonRetryableHandlerError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
