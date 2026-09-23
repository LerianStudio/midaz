// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

//go:generate mockgen -source=context_policy_binding_repository.go -destination=mocks/context_policy_binding_repository_mock.go -package=mocks

// ContextPolicyBindingRepository reads immutable revisions before compilation
// and serializes binding changes within the command's audited transaction.
type ContextPolicyBindingRepository interface {
	GetRevision(context.Context, model.PolicyRevision) (*model.ContextPolicy, error)
	GetBindingWithTx(context.Context, pgdb.Tx, model.PolicyBindingKey) (*model.PolicyBindingState, error)
	BindWithTx(context.Context, pgdb.Tx, model.PolicyBindingKey, model.PolicyRevision, *int64, string, time.Time) error
}
