// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/model"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// GetDataSourceInformation getting all data sources information connected on reporter.
// When a DataSourceProvider is set, it delegates to provider.ListDataSources() and
// maps the result to the existing model.DataSourceInformation format for backward
// compatibility. Otherwise, falls back to the legacy ExternalDataSources map iteration.
func (uc *UseCase) GetDataSourceInformation(ctx context.Context) []*model.DataSourceInformation {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_information")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	if uc.DataSourceProvider != nil {
		return uc.getDataSourceInformationFromProvider(ctx)
	}

	return uc.getDataSourceInformationLegacy(ctx)
}

// getDataSourceInformationFromProvider delegates to the DataSourceProvider interface
// and maps the provider's DataSourceInfo types to model.DataSourceInformation.
func (uc *UseCase) getDataSourceInformationFromProvider(ctx context.Context) []*model.DataSourceInformation {
	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_information_provider")
	defer span.End()

	uc.Logger.Log(ctx, log.LevelInfo, "Listing data sources via DataSourceProvider")

	infos, err := uc.DataSourceProvider.ListDataSources(ctx)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to list data sources from provider", log.Err(err))

		return make([]*model.DataSourceInformation, 0)
	}

	span.SetAttributes(attribute.Int("app.request.external_data_sources.count", len(infos)))

	result := make([]*model.DataSourceInformation, 0, len(infos))

	for _, info := range infos {
		result = append(result, &model.DataSourceInformation{
			Id:           info.ID,
			ExternalName: info.Name,
			Type:         info.Type,
		})
	}

	return result
}

// getDataSourceInformationLegacy is the original implementation that iterates
// ExternalDataSources directly. Retained for backward compatibility when no
// DataSourceProvider is configured.
func (uc *UseCase) getDataSourceInformationLegacy(ctx context.Context) []*model.DataSourceInformation {
	allDataSources := uc.ExternalDataSources.GetAll()

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.Int("app.request.external_data_sources.count", len(allDataSources)))

	uc.Logger.Log(ctx, log.LevelInfo, "Getting data source information (legacy path)", log.Int("datasource_count", len(allDataSources)))

	result := make([]*model.DataSourceInformation, 0)

	for key, dataSource := range allDataSources {
		if !pkg.IsValidDataSourceID(key) {
			uc.Logger.Log(ctx, log.LevelWarn, "Skipping datasource from listing - not in immutable registry (possible corruption)", log.String("datasource", key))
			continue
		}

		var dataSourceInformation *model.DataSourceInformation

		switch dataSource.DatabaseType {
		case "postgresql":
			if dataSource.DatabaseConfig == nil {
				uc.Logger.Log(ctx, log.LevelError, "PostgreSQL datasource has nil DatabaseConfig", log.String("key", key))
				continue
			}

			dataSourceInformation = &model.DataSourceInformation{
				Id:           key,
				ExternalName: dataSource.DatabaseConfig.DBName,
				Type:         dataSource.DatabaseType,
			}
		case "mongodb":
			dataSourceInformation = &model.DataSourceInformation{
				Id:           key,
				ExternalName: dataSource.MongoDBName,
				Type:         dataSource.DatabaseType,
			}
		default:
			uc.Logger.Log(ctx, log.LevelWarn, "Unknown database type", log.String("key", key), log.String("type", dataSource.DatabaseType))
		}

		if dataSourceInformation != nil && strings.TrimSpace(dataSourceInformation.Id) != "" {
			// Add note for plugin_crm about field filtering
			if key == pluginCRMDataSourceID {
				uc.Logger.Log(ctx, log.LevelInfo, "Note: plugin_crm data source filters out encrypted fields and only shows non-encrypted fields and search fields for security")
			}

			result = append(result, dataSourceInformation)
		}
	}

	return result
}
