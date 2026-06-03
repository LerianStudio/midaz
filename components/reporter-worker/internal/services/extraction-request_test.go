// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

// mockExtractionJobCreator implements ExtractionJobCreator for tests.
type mockExtractionJobCreator struct {
	createFunc func(ctx context.Context, jobReq fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error)
}

func (m *mockExtractionJobCreator) CreateExtractionJob(ctx context.Context, jobReq fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
	return m.createFunc(ctx, jobReq)
}

func TestUseCase_IsFetcherMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		client   ExtractionJobCreator
		expected bool
	}{
		{
			name:     "fetcher mode when client is set",
			client:   &mockExtractionJobCreator{},
			expected: true,
		},
		{
			name:     "direct mode when client is nil",
			client:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := &UseCase{FetcherClient: tt.client}
			assert.Equal(t, tt.expected, uc.isFetcherMode())
		})
	}
}

func TestUseCase_RequestFetcherExtraction_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	jobResp := &fetcher.ExtractionJobResponse{
		JobID:     "fetcher-job-001",
		Status:    "accepted",
		CreatedAt: time.Now(),
	}

	mockFetcher := &mockExtractionJobCreator{
		createFunc: func(_ context.Context, jobReq fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
			assert.Contains(t, jobReq.DataRequest.MappedFields, "onboarding")
			assert.Contains(t, jobReq.DataRequest.MappedFields["onboarding"]["organization"], "name")
			assert.Equal(t, reportID.String(), jobReq.Metadata["reportId"])
			assert.Equal(t, templateID.String(), jobReq.Metadata["templateId"])
			return jobResp, nil
		},
	}

	mockExtractionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, mapping *datasource.ExtractionMapping) error {
			assert.Equal(t, "fetcher-job-001", mapping.JobID)
			assert.Equal(t, reportID.String(), mapping.ReportID)
			assert.Equal(t, templateID.String(), mapping.TemplateID)
			assert.Equal(t, "pending", mapping.Status)
			return nil
		})

	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "PendingExtraction", reportID, gomock.Any(), nil).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		FetcherClient:         mockFetcher,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
	}

	err := uc.requestFetcherExtraction(context.Background(), message, &span)
	require.NoError(t, err)
}

func TestUseCase_RequestFetcherExtraction_FetcherClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mockFetcher := &mockExtractionJobCreator{
		createFunc: func(_ context.Context, _ fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
			return nil, errors.New("fetcher service unavailable")
		},
	}

	// handleErrorWithUpdate calls updateReportWithErrors
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	uc := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         tracer,
		FetcherClient:  mockFetcher,
		ReportDataRepo: mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
	}

	err := uc.requestFetcherExtraction(context.Background(), message, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetcher extraction job creation failed")
}

func TestUseCase_RequestFetcherExtraction_MappingSaveError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mockFetcher := &mockExtractionJobCreator{
		createFunc: func(_ context.Context, _ fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
			return &fetcher.ExtractionJobResponse{
				JobID:     "fetcher-job-002",
				Status:    "accepted",
				CreatedAt: time.Now(),
			}, nil
		},
	}

	mockExtractionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(errors.New("mongo connection lost"))

	// handleErrorWithUpdate calls updateReportWithErrors
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		FetcherClient:         mockFetcher,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
	}

	err := uc.requestFetcherExtraction(context.Background(), message, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save extraction mapping")
}

func TestUseCase_RequestFetcherExtraction_StatusUpdateError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mockFetcher := &mockExtractionJobCreator{
		createFunc: func(_ context.Context, _ fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
			return &fetcher.ExtractionJobResponse{
				JobID:     "fetcher-job-003",
				Status:    "accepted",
				CreatedAt: time.Now(),
			}, nil
		},
	}

	mockExtractionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil)

	// First call: UpdateReportStatusById for PendingExtraction fails
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "PendingExtraction", reportID, gomock.Any(), nil).
		Return(errors.New("database write timeout"))

	// Second call: handleErrorWithUpdate calls updateReportWithErrors
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		FetcherClient:         mockFetcher,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
	}

	err := uc.requestFetcherExtraction(context.Background(), message, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database write timeout")
}
