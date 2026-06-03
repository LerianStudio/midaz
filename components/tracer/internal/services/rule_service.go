// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// RuleService is a facade that combines rule commands and queries.
// It implements the RuleService interface expected by the HTTP handler.
type RuleService struct {
	createCmd     *command.CreateRuleCommand
	updateCmd     *command.UpdateRuleCommand
	activateCmd   *command.ActivateRuleService
	deactivateCmd *command.DeactivateRuleService
	draftCmd      *command.DraftRuleService
	deleteCmd     *command.DeleteRuleService
	getQuery      *query.GetRuleQuery
	listQuery     *query.ListRulesQuery
}

// NewRuleService creates a new rule service facade.
func NewRuleService(
	createCmd *command.CreateRuleCommand,
	updateCmd *command.UpdateRuleCommand,
	activateCmd *command.ActivateRuleService,
	deactivateCmd *command.DeactivateRuleService,
	draftCmd *command.DraftRuleService,
	deleteCmd *command.DeleteRuleService,
	getQuery *query.GetRuleQuery,
	listQuery *query.ListRulesQuery,
) *RuleService {
	return &RuleService{
		createCmd:     createCmd,
		updateCmd:     updateCmd,
		activateCmd:   activateCmd,
		deactivateCmd: deactivateCmd,
		draftCmd:      draftCmd,
		deleteCmd:     deleteCmd,
		getQuery:      getQuery,
		listQuery:     listQuery,
	}
}

// CreateRule creates a new rule.
func (s *RuleService) CreateRule(ctx context.Context, input *command.CreateRuleInput) (*model.Rule, error) {
	return s.createCmd.Execute(ctx, input)
}

// UpdateRule updates an existing rule.
func (s *RuleService) UpdateRule(ctx context.Context, id uuid.UUID, input *command.UpdateRuleInput) (*model.Rule, error) {
	return s.updateCmd.Execute(ctx, id, input)
}

// GetRule retrieves a rule by ID.
func (s *RuleService) GetRule(ctx context.Context, id uuid.UUID) (*model.Rule, error) {
	return s.getQuery.Execute(ctx, id)
}

// ListRules retrieves rules with filters.
func (s *RuleService) ListRules(ctx context.Context, filter *model.ListRulesFilter) (*model.ListRulesResult, error) {
	return s.listQuery.Execute(ctx, filter)
}

// ActivateRule activates a rule (DRAFT/INACTIVE → ACTIVE).
// Returns the updated rule for atomic activate-and-return pattern.
func (s *RuleService) ActivateRule(ctx context.Context, id uuid.UUID) (*model.Rule, error) {
	return s.activateCmd.Execute(ctx, id)
}

// DeactivateRule deactivates a rule (ACTIVE/DRAFT → INACTIVE).
// Returns the updated rule for atomic deactivate-and-return pattern.
func (s *RuleService) DeactivateRule(ctx context.Context, id uuid.UUID) (*model.Rule, error) {
	return s.deactivateCmd.Execute(ctx, id)
}

// DraftRule transitions a rule to draft (INACTIVE → DRAFT).
// Returns the updated rule for atomic draft-and-return pattern.
func (s *RuleService) DraftRule(ctx context.Context, id uuid.UUID) (*model.Rule, error) {
	return s.draftCmd.Execute(ctx, id)
}

// DeleteRule soft-deletes a rule (INACTIVE → DELETED).
func (s *RuleService) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return s.deleteCmd.Execute(ctx, id)
}
