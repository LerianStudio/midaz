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

//go:generate mockgen -source=context_policy_repository.go -destination=mocks/context_policy_repository_mock.go -package=mocks

// ContextPolicyPublisher persists a compiled revision inside the command's
// transaction. Publication does not activate or change a context binding.
type ContextPolicyPublisher interface {
	PublishWithTx(context.Context, pgdb.Tx, model.ContextPolicy, string, time.Time) error
}
