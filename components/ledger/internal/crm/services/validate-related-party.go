// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

var validRelatedPartyRoles = map[string]bool{
	"PRIMARY_HOLDER":       true,
	"LEGAL_REPRESENTATIVE": true,
	"RESPONSIBLE_PARTY":    true,
}

func (uc *UseCase) ValidateRelatedParty(ctx context.Context, party *mmodel.RelatedParty) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.validate_related_party")
	defer span.End()

	if party == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Related party payload is nil", cn.ErrInvalidRelatedPartyRole)
		return pkg.ValidateBusinessError(cn.ErrInvalidRelatedPartyRole, cn.EntityRelatedParty)
	}

	if strings.TrimSpace(party.Document) == "" {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Related party document is required", cn.ErrRelatedPartyDocumentRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyDocumentRequired, cn.EntityRelatedParty)
	}

	if strings.TrimSpace(party.Name) == "" {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Related party name is required", cn.ErrRelatedPartyNameRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyNameRequired, cn.EntityRelatedParty)
	}

	if strings.TrimSpace(party.Role) == "" || !validRelatedPartyRoles[party.Role] {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid related party role", cn.ErrInvalidRelatedPartyRole)
		return pkg.ValidateBusinessError(cn.ErrInvalidRelatedPartyRole, cn.EntityRelatedParty)
	}

	if party.StartDate.IsZero() {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Related party start date is required", cn.ErrRelatedPartyStartDateRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyStartDateRequired, cn.EntityRelatedParty)
	}

	if party.EndDate != nil && !party.EndDate.IsZero() && party.EndDate.Before(party.StartDate) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "End date must be after start date", cn.ErrRelatedPartyEndDateInvalid)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyEndDateInvalid, cn.EntityRelatedParty)
	}

	return nil
}

func (uc *UseCase) ValidateRelatedParties(ctx context.Context, parties []*mmodel.RelatedParty) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_related_parties")
	defer span.End()

	for _, party := range parties {
		if err := uc.ValidateRelatedParty(ctx, party); err != nil {
			return err
		}
	}

	return nil
}
