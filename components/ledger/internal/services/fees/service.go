// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
)

// UseCase is a struct to implement the services methods.
// Fields are private to enforce construction through NewUseCase,
// which validates that all required dependencies are provided.
type UseCase struct {
	// packageRepo provides an abstraction on top of the pack data source.
	packageRepo pack.Repository

	// midazClient communicates with midaz
	midazClient http.MidazClient

	// defaultCurrency is the default currency for fee calculations
	defaultCurrency string
}

// ErrNilPackageRepo is returned when a nil PackageRepo is provided to NewUseCase.
var ErrNilPackageRepo = errors.New("PackageRepo is required and cannot be nil")

// ErrNilMidazClient is returned when a nil MidazClient is provided to NewUseCase.
var ErrNilMidazClient = errors.New("MidazClient is required and cannot be nil")

// ErrEmptyDefaultCurrency is returned when an empty DefaultCurrency is provided to NewUseCase.
var ErrEmptyDefaultCurrency = errors.New("DefaultCurrency is required and cannot be empty")

// NewUseCase creates a new UseCase with validated dependencies.
// Returns an error if any required dependency is nil or empty.
func NewUseCase(packageRepo pack.Repository, midazClient http.MidazClient, defaultCurrency string) (*UseCase, error) {
	if packageRepo == nil {
		return nil, ErrNilPackageRepo
	}

	if midazClient == nil {
		return nil, ErrNilMidazClient
	}

	if defaultCurrency == "" {
		return nil, ErrEmptyDefaultCurrency
	}

	return &UseCase{
		packageRepo:     packageRepo,
		midazClient:     midazClient,
		defaultCurrency: defaultCurrency,
	}, nil
}

// PackageRepo returns the package repository dependency.
func (uc *UseCase) PackageRepo() pack.Repository {
	return uc.packageRepo
}

// MidazClient returns the Midaz client dependency.
func (uc *UseCase) MidazClient() http.MidazClient {
	return uc.midazClient
}

// DefaultCurrency returns the default currency for fee calculations.
func (uc *UseCase) DefaultCurrency() string {
	return uc.defaultCurrency
}
