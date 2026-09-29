// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// allowRuleEvaluator allows every request without matching any rule, for tests
// that exercise only the limit side of a reserve.
type allowRuleEvaluator struct{}

func (allowRuleEvaluator) Execute(context.Context, *model.ValidationRequest) (*model.EvaluationResult, error) {
	return &model.EvaluationResult{Decision: model.DecisionAllow}, nil
}
