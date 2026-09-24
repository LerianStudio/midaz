// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=tracer_activation.go -destination=mock_tracer_activation_test.go -package=command

// TracerActivationVerifier belongs to the composed context integration. It must
// confirm local contract/deployment readiness and durable recovery before rules
// can be enabled. It must not create a financial reservation to probe support.
// The transaction client separately verifies the peer's executed-control echo.
type TracerActivationVerifier interface {
	ValidateActivation(ctx context.Context) error
}

func (uc *UseCase) validateTracerActivation(ctx context.Context, settings mmodel.TracerSettings) error {
	if settings.Mode == "" || settings.Mode == mmodel.TracerModeOff || settings.ValidationMode != string(tracercontract.ValidationRulesAndLimits) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if uc.TracerActivation == nil {
		return pkg.ValidateBusinessError(constant.ErrTracerContractUnavailable, constant.EntityLedger)
	}

	return uc.TracerActivation.ValidateActivation(ctx)
}
