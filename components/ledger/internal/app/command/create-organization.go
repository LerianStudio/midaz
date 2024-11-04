package command

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
)

// CreateOrganization creates a new organization persists data in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, tracer trace.Tracer, coi *o.CreateOrganizationInput) (*o.Organization, error) {
	ctx, span := tracer.Start(ctx, reflect.TypeOf(uc).PkgPath()+".CreateOrganization")
	defer span.End()

	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create organization: %v", coi)

	var status o.Status
	if coi.Status.IsEmpty() || common.IsNilOrEmpty(&coi.Status.Code) {
		status = o.Status{
			Code: "ACTIVE",
		}
	} else {
		status = coi.Status
	}

	status.Description = coi.Status.Description

	if common.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	if err := common.ValidateCountryAddress(coi.Address.Country); err != nil {
		span.SetStatus(codes.Error, "Failed to validate country address: "+err.Error())
		span.RecordError(err)

		return nil, common.ValidateBusinessError(err, reflect.TypeOf(o.Organization{}).Name())
	}

	organization := &o.Organization{
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	organizationStr, err := common.StructToJSONString(organization)
	if err != nil {
		span.SetStatus(codes.Error, "Failed to convert organization repository input to JSON string: "+err.Error())
		span.RecordError(err)

		return nil, err
	}

	span.SetAttributes(attribute.KeyValue{
		Key:   attribute.Key("organization_repository_input"),
		Value: attribute.StringValue(organizationStr),
	})

	org, err := uc.OrganizationRepo.Create(ctx, tracer, organization)
	if err != nil {
		span.SetStatus(codes.Error, "Failed to create organization on repository: "+err.Error())
		span.RecordError(err)

		logger.Errorf("Error creating organization: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(o.Organization{}).Name(), org.ID, coi.Metadata)
	if err != nil {
		span.SetStatus(codes.Error, "Failed to create organization metadata: "+err.Error())
		span.RecordError(err)

		logger.Errorf("Error creating organization metadata: %v", err)

		return nil, err
	}

	org.Metadata = metadata

	//TODO: verify if this is necessary
	span.SetStatus(codes.Ok, "Successfully created organization 🎉🚀")

	return org, nil
}
