// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestUseCase_StreamingFieldAssignable verifies the fee UseCase exposes an
// assignable Streaming emitter field (nil disables event emission).
func TestUseCase_StreamingFieldAssignable(t *testing.T) {
	uc := &UseCase{}
	uc.Streaming = pkgStreaming.NewMockEmitter()

	if uc.Streaming == nil {
		t.Fatal("expected UseCase.Streaming to be non-nil after assignment")
	}
}

// TestBillingPackageService_StreamingFieldAssignable verifies the
// BillingPackageService exposes an assignable Streaming emitter field.
func TestBillingPackageService_StreamingFieldAssignable(t *testing.T) {
	s := &BillingPackageService{}
	s.Streaming = pkgStreaming.NewMockEmitter()

	if s.Streaming == nil {
		t.Fatal("expected BillingPackageService.Streaming to be non-nil after assignment")
	}
}
