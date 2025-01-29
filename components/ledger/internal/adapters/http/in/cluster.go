package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ClusterHandler struct contains a cluster use case for managing cluster related operations.
type ClusterHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateCluster is a method that creates cluster information.
//
//	@Summary		Create a Cluster
//	@Description	Create a Cluster with the input payload
//	@Tags			Clusters
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			cluster			body		mmodel.CreateClusterInput	true	"Cluster"
//	@Success		200				{object}	mmodel.Cluster
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/clusters [post]
func (handler *ClusterHandler) CreateCluster(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_cluster")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Cluster with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	payload := i.(*mmodel.CreateClusterInput)
	logger.Infof("Request to create a Cluster with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	cluster, err := handler.Command.CreateCluster(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Cluster on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Cluster")

	return http.Created(c, cluster)
}

// GetAllClusters is a method that retrieves all Clusters.
//
//	@Summary		Get all Clusters
//	@Description	Get all Clusters with the input metadata or without metadata
//	@Tags			Clusters
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			metadata		query		string	false	"Metadata"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			page			query		int		false	"Page"			default(1)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Cluster,page=int,limit=int}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/clusters [get]
func (handler *ClusterHandler) GetAllClusters(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_clusters")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Clusters with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := mpostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Clusters by metadata")

		clusters, err := handler.Query.GetAllMetadataClusters(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Clusters on query", err)

			logger.Errorf("Failed to retrieve all Clusters, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Clusters by metadata")

		pagination.SetItems(clusters)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Clusters ")

	headerParams.Metadata = &bson.M{}

	clusters, err := handler.Query.GetAllClusters(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Clusters on query", err)

		logger.Errorf("Failed to retrieve all Clusters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Clusters")

	pagination.SetItems(clusters)

	return http.OK(c, pagination)
}

// GetClusterByID is a method that retrieves Cluster information by a given id.
//
//	@Summary		Get a Cluster by ID
//	@Description	Get a Cluster with the input ID
//	@Tags			Clusters
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			id				path		string	true	"Cluster ID"
//	@Success		200				{object}	mmodel.Cluster
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/clusters/{id} [get]
func (handler *ClusterHandler) GetClusterByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_cluster_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	cluster, err := handler.Query.GetClusterByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Cluster on query", err)

		logger.Errorf("Failed to retrieve Cluster with Ledger ID: %s and Cluster ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, cluster)
}

// UpdateCluster is a method that updates Cluster information.
//
//	@Summary		Update a Cluster
//	@Description	Update a Cluster with the input payload
//	@Tags			Clusters
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			id				path		string						true	"Cluster ID"
//	@Param			cluster			body		mmodel.UpdateClusterInput	true	"Cluster"
//	@Success		200				{object}	mmodel.Cluster
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/clusters/{id} [patch]
func (handler *ClusterHandler) UpdateCluster(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_cluster")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdateClusterInput)
	logger.Infof("Request to update an Cluster with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateClusterByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Cluster on command", err)

		logger.Errorf("Failed to update Cluster with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	cluster, err := handler.Query.GetClusterByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Cluster on query", err)

		logger.Errorf("Failed to retrieve Cluster with Ledger ID: %s and Cluster ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, cluster)
}

// DeleteClusterByID is a method that removes Cluster information by a given ids.
//
//	@Summary		Delete a Cluster by ID
//	@Description	Delete a Cluster with the input ID
//	@Tags			Clusters
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			id				path	string	true	"Cluster ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/clusters/{id} [delete]
func (handler *ClusterHandler) DeleteClusterByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_cluster_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeleteClusterByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Cluster on command", err)

		logger.Errorf("Failed to remove Cluster with Ledger ID: %s and Cluster ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Cluster with Organization ID: %s and Ledger ID: %s and Cluster ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.NoContent(c)
}
