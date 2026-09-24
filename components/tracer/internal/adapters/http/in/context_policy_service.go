// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

//go:generate mockgen -source=context_policy_service.go -destination=mock_context_policy_service_test.go -package=in

// ContextPolicyAdminService is the tenant-scoped administration port.
type ContextPolicyAdminService interface {
	Publish(context.Context, model.ContextPolicy) error
	Bind(context.Context, model.PolicyBindingKey, model.PolicyRevision, *int64) (*model.PolicyBindingState, error)
	GetRevision(context.Context, model.PolicyRevision) (*model.ContextPolicy, error)
	GetBinding(context.Context, model.PolicyBindingKey) (*model.PolicyBindingState, error)
}
