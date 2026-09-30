// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// allowRuleEvaluator allows every request without matching any rule, for
// reservation proofs that exercise only the limit lifecycle.
type allowRuleEvaluator struct{}

func (allowRuleEvaluator) Execute(context.Context, *model.ValidationRequest) (*model.EvaluationResult, error) {
	return &model.EvaluationResult{Decision: model.DecisionAllow}, nil
}
