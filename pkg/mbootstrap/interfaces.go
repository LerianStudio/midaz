// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package mbootstrap provides shared interfaces for bootstrapping Midaz components.
// This package enables composition of multiple components (onboarding, transaction)
// into a unified service (ledger) while maintaining encapsulation of internal implementations.
package mbootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
)

// Runnable represents a component that can be run by the launcher.
// This interface is compatible with libCommons.NewLauncher's RunApp function.
type Runnable interface {
	Run(l *libCommons.Launcher) error
}

// Service represents a bootstrapped service that can be composed into a unified deployment.
// Each component (onboarding, transaction) implements this interface to expose
// its runnable components for composition.
type Service interface {
	// GetRunnables returns all runnable components of this service.
	// These will be passed to libCommons.NewLauncher for execution.
	GetRunnables() []RunnableConfig
}

// RunnableConfig pairs a runnable with its name for the launcher.
type RunnableConfig struct {
	Name     string
	Runnable Runnable
}
