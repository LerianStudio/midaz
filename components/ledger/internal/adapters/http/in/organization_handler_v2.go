// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file holds the /v2 organization CREATE shell. It is the only organization op
// that differs between the two contracts: the self-holder provisioning behind
// command.RouteHolderPolicy is a /v2 seam.
//
// Unlike the account surface, nothing about the organization RESPONSE is versioned —
// mmodel.Organization carries no holder field — so this shell reuses the /v1 request
// and response envelopes verbatim and the two ops publish one schema component. The
// five remaining organization ops (list, get-by-id, update, delete, count) bind the
// same handler methods on both contracts.

// CreateOrganizationV2 decodes+validates the raw body imperatively then delegates to
// the shared createOrganization core under command.HolderOnV2, so the organization's
// deterministic self-holder is provisioned in CRM.
func (handler *OrganizationHandler) CreateOrganizationV2(ctx context.Context, in *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	payload := new(mmodel.CreateOrganizationInput)
	if _, err := pkgHTTP.DecodeAndValidate(in.RawBody, payload); err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	organization, err := handler.createOrganization(ctx, payload, command.HolderOnV2)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &CreateOrganizationResponse{Status: http.StatusCreated, Body: organization}, nil
}
