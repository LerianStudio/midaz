// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// LimitAssetDocument contains official producer facts, never an administrative
// guess based solely on a code. The native TLS identity owns the namespace.
type (
	LimitAssetDocument struct {
		AccountAssets []tracercontract.AccountAsset `json:"accountAssets" nullable:"false" minItems:"1"`
	}
	BindLimitAssetInput struct {
		ID      string `path:"id"`
		RawBody []byte `contentType:"application/json"`
	}
	LimitAssetOutput struct{ Body *tracercontract.AssetRef }
)

type LimitAssetHandler struct {
	binder       LimitAssetBinder
	identity     *seamidentity.Resolver
	bounds       tracercontract.Limits
	maxBodyBytes int
}

func NewLimitAssetHandler(binder LimitAssetBinder, identity *seamidentity.Resolver, bounds tracercontract.Limits, maxBodyBytes int) (*LimitAssetHandler, error) {
	if binder == nil || identity == nil || maxBodyBytes <= 0 {
		return nil, constant.ErrContextLimitsUnavailable
	}

	if err := bounds.Validate(); err != nil {
		return nil, err
	}

	return &LimitAssetHandler{binder: binder, identity: identity, bounds: bounds, maxBodyBytes: maxBodyBytes}, nil
}

func (h *LimitAssetHandler) Bind(ctx context.Context, input *BindLimitAssetInput) (_ *LimitAssetOutput, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.bind_limit_asset")
	defer span.End()
	defer func() {
		if retErr == nil {
			return
		}

		retErr = canonicalLimitAssetError(retErr)
		if pkg.IsBusinessError(retErr) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Asset association rejected", retErr)
		} else {
			libOtel.HandleSpanError(span, "Asset association failed", retErr)
		}

		retErr = humaProblem(retErr)
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	id, err := uuid.Parse(input.ID)
	if err != nil || id == uuid.Nil {
		return nil, constant.ErrInvalidPathParameter
	}

	if len(input.RawBody) > h.maxBodyBytes {
		return nil, constant.ErrInvalidRequestBody
	}

	var body LimitAssetDocument
	if err := decodePolicyBody(input.RawBody, &body); err != nil {
		return nil, err
	}

	if err := tracercontract.ValidateAccountAssets(ctx, body.AccountAssets, identity.AssetNamespace, h.bounds); err != nil {
		return nil, err
	}

	asset, err := h.binder.Execute(ctx, id, body.AccountAssets)
	if err != nil {
		return nil, err
	}

	if asset == nil {
		return nil, constant.ErrInternalServer
	}

	logging.WithTrace(ctx, logger).With(libLog.Int("accounts.count", len(body.AccountAssets))).Log(ctx, libLog.LevelDebug, "Asset administration completed")

	return &LimitAssetOutput{Body: asset}, nil
}

func canonicalLimitAssetError(err error) error {
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		return pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "Invalid official account asset facts."}
	}

	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		mapped := pkg.ValidateBusinessError(cause, constant.EntityLimit)

		var unavailable pkg.ServiceUnavailableError
		if pkg.IsBusinessError(mapped) || errors.As(mapped, &unavailable) {
			return mapped
		}
	}

	return err
}

func RegisterLimitAssetRoutes(api huma.API, h *LimitAssetHandler) {
	if api.OpenAPI().Components.SecuritySchemes == nil {
		api.OpenAPI().Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	api.OpenAPI().Components.SecuritySchemes["ProducerMTLS"] = &huma.SecurityScheme{Type: "mutualTLS", Description: "Verified native client certificate whose URI SAN is registered to the producer namespace."}

	huma.Register(api, huma.Operation{
		OperationID: "bindLimitAsset", Method: http.MethodPut, Path: "/limits/{id}/asset-reference",
		Summary: "Associate a limit with official account asset facts", Tags: []string{"Limits"},
		Description: "Requires a verified producer certificate and Access Manager permission on limit-asset-references. The association is immutable and audited; repeating it returns a conflict.",
		Security:    []map[string][]string{{"BearerAuth": {}, "ProducerMTLS": {}}}, SkipValidateBody: true, MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 404, 409, 413, 503},
	}, h.Bind)
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[LimitAssetDocument](), true, "")
	api.OpenAPI().Paths["/limits/{id}/asset-reference"].Put.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
}
