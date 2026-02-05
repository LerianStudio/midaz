// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var validRelatedPartyRoles = map[string]bool{
	"PRIMARY_HOLDER":       true,
	"LEGAL_REPRESENTATIVE": true,
	"RESPONSIBLE_PARTY":    true,
}

func (uc *UseCase) ValidateRelatedParty(ctx context.Context, party *mmodel.RelatedParty) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.validate_related_party")
	defer span.End()

	logger.Infof("Validating related party: role=%s", party.Role)

	if strings.TrimSpace(party.Document) == "" {
		libOpenTelemetry.HandleSpanError(&span, "Related party document is required", cn.ErrRelatedPartyDocumentRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyDocumentRequired, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	if strings.TrimSpace(party.Name) == "" {
		libOpenTelemetry.HandleSpanError(&span, "Related party name is required", cn.ErrRelatedPartyNameRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyNameRequired, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	if strings.TrimSpace(party.Role) == "" || !validRelatedPartyRoles[party.Role] {
		libOpenTelemetry.HandleSpanError(&span, "Invalid related party role", cn.ErrInvalidRelatedPartyRole)
		return pkg.ValidateBusinessError(cn.ErrInvalidRelatedPartyRole, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	if party.StartDate.IsZero() {
		libOpenTelemetry.HandleSpanError(&span, "Related party start date is required", cn.ErrRelatedPartyStartDateRequired)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyStartDateRequired, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	if party.EndDate != nil && !party.EndDate.IsZero() && party.EndDate.Before(party.StartDate) {
		libOpenTelemetry.HandleSpanError(&span, "End date must be after start date", cn.ErrRelatedPartyEndDateInvalid)
		return pkg.ValidateBusinessError(cn.ErrRelatedPartyEndDateInvalid, reflect.TypeOf(mmodel.RelatedParty{}).Name())
	}

	return nil
}

func (uc *UseCase) ValidateRelatedParties(ctx context.Context, parties []*mmodel.RelatedParty) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_related_parties")
	defer span.End()

	logger.Infof("Validating related parties")

	for _, party := range parties {
		if err := uc.ValidateRelatedParty(ctx, party); err != nil {
			return err
		}
	}

	return nil
}
