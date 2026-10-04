// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package scoperesolver translates the values a tracer request names into the
// scope dimensions the authorization service decides on: a validation id into
// the account, segment, portfolio and merchant the validation was submitted for.
package scoperesolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Resolver names the tracer manifest's scope.routes refer to.
const (
	ValidationAccount   = "validationAccount"
	ValidationSegment   = "validationSegment"
	ValidationPortfolio = "validationPortfolio"
	ValidationMerchant  = "validationMerchant"
)

// ValidationReader reads a stored validation by id, answering
// constant.ErrTransactionValidationNotFound for an id that names none.
type ValidationReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error)
}

// TenantAttacher attaches the tenant's database to a context; in single-tenant
// mode it returns the context unchanged.
type TenantAttacher interface {
	Active() bool
	Resolve(ctx context.Context, tenantID string) (context.Context, error)
}

// errNoValidationReader refuses a lookup when no validation reader is wired.
var errNoValidationReader = errors.New("scope resolution of validations has no validation reader configured")

// resolvers holds what the lookups read: the stored validations and, in
// multi-tenant mode, the attachment of the credential's tenant database.
type resolvers struct {
	validations ValidationReader
	tenant      TenantAttacher
}

// Register registers every resolver the tracer manifest names. It must run
// before declaration.WireScope, which refuses a manifest naming a resolver that
// is not registered. A nil tenant is single-tenant mode.
func Register(auth *middleware.AuthClient, validations ValidationReader, tenant TenantAttacher) error {
	for name, fn := range pickers(resolvers{validations: validations, tenant: tenant}) {
		if err := auth.RegisterScopeResolver(name, fn); err != nil {
			return err
		}
	}

	return nil
}

// pickers is every resolver the manifest names, each answering one part of the
// stored validation.
func pickers(set resolvers) map[string]middleware.ScopeResolver {
	return map[string]middleware.ScopeResolver{
		ValidationAccount: set.byValidation(func(v *model.TransactionValidation) uuid.UUID { return v.Account.ID }),
		ValidationSegment: set.byValidation(func(v *model.TransactionValidation) uuid.UUID {
			if v.Segment == nil {
				return uuid.Nil
			}

			return v.Segment.ID
		}),
		ValidationPortfolio: set.byValidation(func(v *model.TransactionValidation) uuid.UUID {
			if v.Portfolio == nil {
				return uuid.Nil
			}

			return v.Portfolio.ID
		}),
		ValidationMerchant: set.byValidation(func(v *model.TransactionValidation) uuid.UUID {
			if v.Merchant == nil {
				return uuid.Nil
			}

			return v.Merchant.ID
		}),
	}
}

// byValidation answers, for each validation id, the part of the stored
// validation pick selects. A value that is not a uuid, a validation that does
// not exist, or one that carries no such part names nothing.
func (s resolvers) byValidation(pick func(*model.TransactionValidation) uuid.UUID) middleware.ScopeResolver {
	return func(ctx context.Context, in middleware.ResolveInput) ([][]string, error) {
		if s.validations == nil {
			return nil, errNoValidationReader
		}

		ctx, err := s.attachTenant(ctx)
		if err != nil {
			return nil, err
		}

		out := make([][]string, len(in.Items))

		for i, item := range in.Items {
			id, err := uuid.Parse(item.Value)
			if err != nil {
				continue
			}

			validation, err := s.validations.GetByID(ctx, id)
			if errors.Is(err, constant.ErrTransactionValidationNotFound) {
				continue
			}

			if err != nil {
				return nil, fmt.Errorf("resolve %s of validation %s: %w", in.Dimension, item.Value, err)
			}

			if part := pick(validation); part != uuid.Nil {
				out[i] = []string{part.String()}
			}
		}

		return out, nil
	}
}

// attachTenant attaches the database of the tenant the validated credential
// names when the deployment is multi-tenant, and leaves the context as it is
// otherwise.
func (s resolvers) attachTenant(ctx context.Context) (context.Context, error) {
	if s.tenant == nil || !s.tenant.Active() {
		return ctx, nil
	}

	var tenantID string

	if principal, ok := middleware.PrincipalFromContext(ctx); ok && principal.TenantID != "" {
		canonical, err := tmcore.CanonicalTenantID(principal.TenantID)
		if err != nil {
			return ctx, fmt.Errorf("scope resolution tenant: %w", err)
		}

		tenantID = canonical
	}

	ctx, err := s.tenant.Resolve(ctx, tenantID)
	if err != nil {
		return ctx, fmt.Errorf("scope resolution tenant database: %w", err)
	}

	return ctx, nil
}
