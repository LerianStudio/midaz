// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"strconv"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ContextPolicyDocument is an immutable policy revision, independent of ledger
// primitives. Publishing does not activate it on any binding.
type ContextPolicyDocument struct {
	ID              uuid.UUID                   `json:"id" format:"uuid"`
	Revision        int64                       `json:"revision" minimum:"1"`
	DefaultDecision model.Decision              `json:"defaultDecision" enum:"ALLOW,DENY" doc:"Explicit default; use DENY for the initial policy."`
	Rules           []ContextPolicyRuleDocument `json:"rules" nullable:"false"`
}

type ContextPolicyRuleDocument struct {
	ID         uuid.UUID      `json:"id" format:"uuid"`
	Revision   int64          `json:"revision" minimum:"1"`
	Expression string         `json:"expression" minLength:"1"`
	Action     model.Decision `json:"action" enum:"ALLOW,DENY,REVIEW"`
}

type BindContextPolicyDocument struct {
	IntegrationID   string    `json:"integrationId" minLength:"1" maxLength:"256"`
	ContextID       string    `json:"contextId" minLength:"1" maxLength:"256"`
	PolicyID        uuid.UUID `json:"policyId" format:"uuid"`
	PolicyRevision  int64     `json:"policyRevision" minimum:"1"`
	ExpectedVersion *int64    `json:"expectedVersion,omitempty" minimum:"1" doc:"Omit only to create a new binding; replacement requires its current version."`
}

type ContextPolicyBindingDocument struct {
	IntegrationID  string    `json:"integrationId"`
	ContextID      string    `json:"contextId"`
	PolicyID       uuid.UUID `json:"policyId" format:"uuid"`
	PolicyRevision int64     `json:"policyRevision"`
	Version        int64     `json:"version"`
}

type (
	PublishContextPolicyInput struct {
		RawBody []byte `contentType:"application/json"`
	}
	ContextPolicyOutput struct {
		Status int
		Body   *ContextPolicyDocument
	}
	GetContextPolicyInput struct {
		ID       string `path:"id"`
		Revision string `path:"revision"`
	}
	BindContextPolicyInput struct {
		RawBody []byte `contentType:"application/json"`
	}
	ContextPolicyBindingOutput   struct{ Body *ContextPolicyBindingDocument }
	GetContextPolicyBindingInput struct {
		IntegrationID string `query:"integrationId"`
		ContextID     string `query:"contextId"`
	}
)

type ContextPolicyHandler struct {
	service      ContextPolicyAdminService
	maxRules     int
	maxBodyBytes int
}

func NewContextPolicyHandler(service ContextPolicyAdminService, maxRules, maxBodyBytes int) (*ContextPolicyHandler, error) {
	if service == nil || maxRules <= 0 || maxBodyBytes <= 0 {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &ContextPolicyHandler{service: service, maxRules: maxRules, maxBodyBytes: maxBodyBytes}, nil
}

func (h *ContextPolicyHandler) Publish(ctx context.Context, input *PublishContextPolicyInput) (*ContextPolicyOutput, error) {
	return contextPolicyRequest(ctx, "handler.policy.publish", func(ctx context.Context) (*ContextPolicyOutput, error) {
		var body ContextPolicyDocument
		if err := decodePolicyBody(input.RawBody, &body); err != nil {
			return nil, err
		}

		if body.Rules == nil {
			return nil, constant.ErrInvalidRequestBody
		}

		policy := body.policy()
		if err := policy.Validate(h.maxRules); err != nil {
			return nil, err
		}

		if err := h.service.Publish(ctx, policy); err != nil {
			return nil, err
		}

		return &ContextPolicyOutput{Status: http.StatusCreated, Body: &body}, nil
	})
}

func (h *ContextPolicyHandler) GetRevision(ctx context.Context, input *GetContextPolicyInput) (*ContextPolicyOutput, error) {
	return contextPolicyRequest(ctx, "handler.policy.get_revision", func(ctx context.Context) (*ContextPolicyOutput, error) {
		id, err := uuid.Parse(input.ID)
		if err != nil || id == uuid.Nil {
			return nil, constant.ErrInvalidPathParameter
		}

		revision, err := strconv.ParseInt(input.Revision, 10, 64)
		if err != nil || revision <= 0 {
			return nil, constant.ErrInvalidPathParameter
		}

		result, err := h.service.GetRevision(ctx, model.PolicyRevision{ID: id, Revision: revision})
		if err != nil {
			return nil, err
		}

		if result == nil {
			return nil, constant.ErrContextPolicyUnavailable
		}

		return &ContextPolicyOutput{Status: http.StatusOK, Body: policyDocument(*result)}, nil
	})
}

func (h *ContextPolicyHandler) Bind(ctx context.Context, input *BindContextPolicyInput) (*ContextPolicyBindingOutput, error) {
	return contextPolicyRequest(ctx, "handler.policy.bind", func(ctx context.Context) (*ContextPolicyBindingOutput, error) {
		var body BindContextPolicyDocument
		if err := decodePolicyBody(input.RawBody, &body); err != nil {
			return nil, err
		}

		key := model.PolicyBindingKey{IntegrationID: body.IntegrationID, ContextID: body.ContextID}
		if err := key.Validate(); err != nil {
			return nil, err
		}

		if body.PolicyID == uuid.Nil || body.PolicyRevision <= 0 {
			return nil, constant.ErrInvalidRequestBody
		}

		if body.ExpectedVersion != nil && (*body.ExpectedVersion <= 0 || *body.ExpectedVersion == math.MaxInt64) {
			return nil, constant.ErrInvalidRequestBody
		}

		result, err := h.service.Bind(ctx, key, model.PolicyRevision{ID: body.PolicyID, Revision: body.PolicyRevision}, body.ExpectedVersion)
		if err != nil {
			return nil, err
		}

		return bindingOutput(key, result)
	})
}

func (h *ContextPolicyHandler) GetBinding(ctx context.Context, input *GetContextPolicyBindingInput) (*ContextPolicyBindingOutput, error) {
	return contextPolicyRequest(ctx, "handler.policy.get_binding", func(ctx context.Context) (*ContextPolicyBindingOutput, error) {
		key := model.PolicyBindingKey{IntegrationID: input.IntegrationID, ContextID: input.ContextID}
		if err := key.Validate(); err != nil {
			return nil, constant.ErrInvalidQueryParameter
		}

		result, err := h.service.GetBinding(ctx, key)
		if err != nil {
			return nil, err
		}

		return bindingOutput(key, result)
	})
}

func bindingOutput(key model.PolicyBindingKey, state *model.PolicyBindingState) (*ContextPolicyBindingOutput, error) {
	if state == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &ContextPolicyBindingOutput{Body: &ContextPolicyBindingDocument{IntegrationID: key.IntegrationID, ContextID: key.ContextID, PolicyID: state.Policy.ID, PolicyRevision: state.Policy.Revision, Version: state.Version}}, nil
}

func (d ContextPolicyDocument) policy() model.ContextPolicy {
	rules := make([]model.ContextPolicyRule, len(d.Rules))
	for i, r := range d.Rules {
		rules[i] = model.ContextPolicyRule{ID: r.ID, Revision: r.Revision, Expression: r.Expression, Action: r.Action}
	}

	return model.ContextPolicy{ID: d.ID, Revision: d.Revision, DefaultDecision: d.DefaultDecision, Rules: rules}
}

func policyDocument(p model.ContextPolicy) *ContextPolicyDocument {
	rules := make([]ContextPolicyRuleDocument, len(p.Rules))
	for i, r := range p.Rules {
		rules[i] = ContextPolicyRuleDocument{ID: r.ID, Revision: r.Revision, Expression: r.Expression, Action: r.Action}
	}

	return &ContextPolicyDocument{ID: p.ID, Revision: p.Revision, DefaultDecision: p.DefaultDecision, Rules: rules}
}

func decodePolicyBody(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return constant.ErrInvalidRequestBody
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

func contextPolicyRequest[T any](ctx context.Context, name string, fn func(context.Context) (*T, error)) (*T, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, name)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	var result *T

	err := ctx.Err()
	if err == nil {
		result, err = fn(ctx)
	}

	if err != nil {
		err = canonicalPolicyError(err)
		if pkg.IsBusinessError(err) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Policy administration rejected", err)
		} else {
			libOtel.HandleSpanError(span, "Policy administration failed", err)
		}

		return nil, humaProblem(err)
	}

	logger.Log(ctx, libLog.LevelDebug, "Policy administration request completed")

	return result, nil
}

func canonicalPolicyError(err error) error {
	if errors.Is(err, constant.ErrInvalidRequestBody) {
		return pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), Title: "Bad Request", Message: "Invalid policy request body."}
	}

	if errors.Is(err, constant.ErrContextPolicyUnavailable) {
		return pkg.ValidateBusinessError(constant.ErrContextPolicyUnavailable, constant.EntityContextPolicy)
	}

	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		mapped := pkg.ValidateBusinessError(cause, constant.EntityContextPolicy)
		if pkg.IsBusinessError(mapped) {
			return mapped
		}
	}

	return err
}

// RegisterContextPolicyRoutes registers only the contract. The shared route
// mount must attach plugin authorization before these handlers.
func RegisterContextPolicyRoutes(api huma.API, h *ContextPolicyHandler) {
	api.OpenAPI().Tags = append(api.OpenAPI().Tags, &huma.Tag{
		Name: "Policies", Description: "Immutable evaluation policies and tenant-scoped integration bindings.",
	})

	body := func(value any) *huma.RequestBody {
		return &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
			"application/json": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeOf(value), true, "")},
		}}
	}
	security := []map[string][]string{{"BearerAuth": {}}}
	huma.Register(api, huma.Operation{
		OperationID: "publishContextPolicy", Method: http.MethodPost, Path: "/policies",
		DefaultStatus: http.StatusCreated, Summary: "Publish an immutable policy revision",
		Tags: []string{"Policies"}, Security: security, SkipValidateBody: true,
		MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 409, 413, 503},
	}, h.Publish)
	huma.Register(api, huma.Operation{
		OperationID: "getContextPolicyRevision", Method: http.MethodGet, Path: "/policies/{id}/revisions/{revision}",
		Summary: "Get a published policy revision", Tags: []string{"Policies"}, Security: security,
		Errors: []int{400, 401, 403, 503},
	}, h.GetRevision)
	huma.Register(api, huma.Operation{
		OperationID: "bindContextPolicy", Method: http.MethodPut, Path: "/policy-bindings",
		Summary: "Activate a policy on an exact context binding", Tags: []string{"Policies"}, Security: security,
		SkipValidateBody: true, MaxBodyBytes: int64(h.maxBodyBytes), Errors: []int{400, 401, 403, 409, 413, 503},
	}, h.Bind)
	huma.Register(api, huma.Operation{
		OperationID: "getContextPolicyBinding", Method: http.MethodGet, Path: "/policy-bindings",
		Summary: "Get the active policy binding and version", Tags: []string{"Policies"}, Security: security,
		Errors: []int{400, 401, 403, 503},
	}, h.GetBinding)
	// RawBody registration derives a binary schema. Publish the actual JSON DTO
	// after registration; runtime validation remains in the strict decoder.
	api.OpenAPI().Paths["/policies"].Post.RequestBody = body(ContextPolicyDocument{})
	api.OpenAPI().Paths["/policy-bindings"].Put.RequestBody = body(BindContextPolicyDocument{})
}
