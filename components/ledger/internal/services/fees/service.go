// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	pkg "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
)

// UseCase is a struct to implement the services methods.
// Fields are private to enforce construction through NewUseCase,
// which validates that all required dependencies are provided.
type UseCase struct {
	// packageRepo provides an abstraction on top of the pack data source.
	packageRepo pack.Repository

	// resolver resolves account/transaction reads in-process via the ledger query layer.
	resolver pkg.MidazResolver

	// defaultCurrency is the default currency for fee calculations
	defaultCurrency string
}

// ErrNilPackageRepo is returned when a nil PackageRepo is provided to NewUseCase.
var ErrNilPackageRepo = errors.New("PackageRepo is required and cannot be nil")

// ErrNilResolver is returned when a nil MidazResolver is provided to NewUseCase.
var ErrNilResolver = errors.New("MidazResolver is required and cannot be nil")

// ErrEmptyDefaultCurrency is returned when an empty DefaultCurrency is provided to NewUseCase.
var ErrEmptyDefaultCurrency = errors.New("DefaultCurrency is required and cannot be empty")

// NewUseCase creates a new UseCase with validated dependencies.
// Returns an error if any required dependency is nil or empty.
func NewUseCase(packageRepo pack.Repository, resolver pkg.MidazResolver, defaultCurrency string) (*UseCase, error) {
	if packageRepo == nil {
		return nil, ErrNilPackageRepo
	}

	if resolver == nil {
		return nil, ErrNilResolver
	}

	if defaultCurrency == "" {
		return nil, ErrEmptyDefaultCurrency
	}

	return &UseCase{
		packageRepo:     packageRepo,
		resolver:        resolver,
		defaultCurrency: defaultCurrency,
	}, nil
}

// PackageRepo returns the package repository dependency.
func (uc *UseCase) PackageRepo() pack.Repository {
	return uc.packageRepo
}

// Resolver returns the in-process Midaz resolver dependency.
func (uc *UseCase) Resolver() pkg.MidazResolver {
	return uc.resolver
}

// DefaultCurrency returns the default currency for fee calculations.
func (uc *UseCase) DefaultCurrency() string {
	return uc.defaultCurrency
}
