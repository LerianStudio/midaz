// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ContextPolicyReader selects immutable revisions or an exact tenant binding.
type ContextPolicyReader interface {
	GetRevision(context.Context, model.PolicyRevision) (*model.ContextPolicy, error)
	GetActive(context.Context, model.PolicyBindingKey) (*model.BoundContextPolicy, error)
}

// ContextPolicyService composes the administrative commands and read ports.
// Request authentication and tenant resolution belong to the transport.
type ContextPolicyService struct {
	publisher *command.PublishContextPolicyCommand
	binder    *command.BindContextPolicyCommand
	reader    ContextPolicyReader
}

func NewContextPolicyService(publisher *command.PublishContextPolicyCommand, binder *command.BindContextPolicyCommand, reader ContextPolicyReader) (*ContextPolicyService, error) {
	if publisher == nil || binder == nil || reader == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &ContextPolicyService{publisher: publisher, binder: binder, reader: reader}, nil
}

func (s *ContextPolicyService) Publish(ctx context.Context, policy model.ContextPolicy) error {
	return s.publisher.Execute(ctx, policy)
}

func (s *ContextPolicyService) Bind(ctx context.Context, key model.PolicyBindingKey, target model.PolicyRevision, expected *int64) (*model.PolicyBindingState, error) {
	return s.binder.Execute(ctx, key, target, expected)
}

func (s *ContextPolicyService) GetRevision(ctx context.Context, ref model.PolicyRevision) (*model.ContextPolicy, error) {
	return s.reader.GetRevision(ctx, ref)
}

func (s *ContextPolicyService) GetBinding(ctx context.Context, key model.PolicyBindingKey) (*model.PolicyBindingState, error) {
	snapshot, err := s.reader.GetActive(ctx, key)
	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		return nil, constant.ErrContextPolicyUnavailable
	}

	return &model.PolicyBindingState{Policy: model.PolicyRevision{ID: snapshot.ID, Revision: snapshot.Revision}, Version: snapshot.BindingVersion}, nil
}
