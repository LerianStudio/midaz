// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Authorizer provides write-path authorization via external gRPC service.
type Authorizer interface {
	Enabled() bool
	Authorize(ctx context.Context, req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error)
	LoadBalances(ctx context.Context, req *authorizerv1.LoadBalancesRequest) (*authorizerv1.LoadBalancesResponse, error)
}
