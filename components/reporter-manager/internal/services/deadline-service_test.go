// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestUseCase_CreateDeadline(t *testing.T) {
	t.Parallel()

	dueDate := time.Now().Add(24 * time.Hour)
	templateID := uuid.New()

	tests := []struct {
		name           string
		input          *deadline.CreateDeadlineInput
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult bool
	}{
		{
			name: "Success - Create a deadline without templateId",
			input: &deadline.CreateDeadlineInput{
				Name:      "Monthly Regulatory Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&deadline.Deadline{
						ID:        uuid.New(),
						Name:      "Monthly Regulatory Report",
						Type:      "regulatory",
						DueDate:   dueDate,
						Frequency: "monthly",
						Color:     "#FF5733",
						Active:    true,
						Status:    deadline.StatusPending,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Create a deadline with templateId fills templateName",
			input: &deadline.CreateDeadlineInput{
				Name:       "Monthly Regulatory Report",
				Type:       "regulatory",
				TemplateID: &templateID,
				DueDate:    dueDate,
				Frequency:  "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockTemplateRepo := template.NewMockRepository(ctrl)

				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(&template.Template{
						ID:           templateID,
						Description:  "Template Financeiro",
						OutputFormat: "pdf",
					}, nil)

				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&deadline.Deadline{
						ID:           uuid.New(),
						Name:         "Monthly Regulatory Report",
						Type:         "regulatory",
						TemplateID:   &templateID,
						TemplateName: "Template Financeiro",
						DueDate:      dueDate,
						Frequency:    "monthly",
						Color:        "#FF5733",
						Active:       true,
						Status:       deadline.StatusPending,
						CreatedAt:    time.Now(),
						UpdatedAt:    time.Now(),
					}, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
					TemplateRepo: mockTemplateRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Create a deadline with explicit active=false",
			input: &deadline.CreateDeadlineInput{
				Name:      "Inactive Deadline",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color:  "#FF5733",
				Active: func() *bool { b := false; return &b }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&deadline.Deadline{
						ID:        uuid.New(),
						Name:      "Inactive Deadline",
						Type:      "regulatory",
						DueDate:   dueDate,
						Frequency: "monthly",
						Color:     "#FF5733",
						Active:    false,
						Status:    deadline.StatusPending,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Error - Create deadline with invalid frequency",
			input: &deadline.CreateDeadlineInput{
				Name:      "Bad Deadline",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "invalid_freq",
				Color:     "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrInvalidDeadlineFrequency.Error(),
		},
		{
			name: "Error - Create deadline with monthsOfYear on weekly frequency",
			input: &deadline.CreateDeadlineInput{
				Name:         "Weekly Report",
				Type:         "regulatory",
				DueDate:      dueDate,
				Frequency:    "weekly",
				Color:        "#FF5733",
				MonthsOfYear: []int{1, 6},
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearNotApplicable.Error(),
		},
		{
			name: "Error - Create deadline with monthsOfYear on monthly frequency",
			input: &deadline.CreateDeadlineInput{
				Name:      "Monthly Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color:        "#FF5733",
				MonthsOfYear: []int{1, 6},
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearNotApplicable.Error(),
		},
		{
			name: "Error - Create deadline with annual frequency missing monthsOfYear",
			input: &deadline.CreateDeadlineInput{
				Name:      "Annual Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "annual",
				Color:     "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearRequired.Error(),
		},
		{
			name: "Error - Create deadline with semiannual frequency missing monthsOfYear",
			input: &deadline.CreateDeadlineInput{
				Name:      "Semiannual Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "semiannual",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearRequired.Error(),
		},
		{
			name: "Error - Create deadline with monthsOfYear containing 0",
			input: &deadline.CreateDeadlineInput{
				Name:      "Semiannual Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "semiannual",

				MonthsOfYear: []int{0, 6},
				Color:        "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearOutOfRange.Error(),
		},
		{
			name: "Error - Create deadline with monthsOfYear containing 13",
			input: &deadline.CreateDeadlineInput{
				Name:      "Semiannual Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "semiannual",

				MonthsOfYear: []int{1, 13},
				Color:        "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearOutOfRange.Error(),
		},
		{
			name: "Error - Create deadline with invalid type",
			input: &deadline.CreateDeadlineInput{
				Name:      "Bad Deadline",
				Type:      "invalid_type",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrInvalidDeadlineType.Error(),
		},
		{
			name: "Error - Create deadline with missing name",
			input: &deadline.CreateDeadlineInput{
				Name:      "",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			},
			expectErr:   true,
			errContains: constant.ErrMissingRequiredFields.Error(),
		},
		{
			name: "Error - Repository Create fails",
			input: &deadline.CreateDeadlineInput{
				Name:      "Monthly Regulatory Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name: "Error - Template not found when templateId provided",
			input: &deadline.CreateDeadlineInput{
				Name:       "Monthly Regulatory Report",
				Type:       "regulatory",
				TemplateID: &templateID,
				DueDate:    dueDate,
				Frequency:  "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockTemplateRepo := template.NewMockRepository(ctrl)

				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(nil, mongo.ErrNoDocuments)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
					TemplateRepo: mockTemplateRepo,
				}
			},
			expectErr:   true,
			errContains: "template",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, err := svc.CreateDeadline(ctx, tt.input)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.NotEqual(t, uuid.Nil, result.ID)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Type, result.Type)
				assert.Equal(t, tt.input.Frequency, result.Frequency)
				assert.Equal(t, tt.input.Color, result.Color)
			}
		})
	}
}

func TestUseCase_GetAllDeadlines(t *testing.T) {
	t.Parallel()

	dueDate := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name          string
		filters       http.QueryHeader
		mockSetup     func(ctrl *gomock.Controller) *UseCase
		expectErr     bool
		errContains   string
		expectedCount int
		expectedTotal int64
	}{
		{
			name: "Success - Get all deadlines with default filters",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{
						{
							ID:        uuid.New(),
							Name:      "Monthly Report",
							Type:      "regulatory",
							DueDate:   dueDate,
							Frequency: "monthly",
							Color:     "#FF5733",
							Active:    true,
							Status:    deadline.StatusPending,
						},
					}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(5), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:     false,
			expectedCount: 1,
			expectedTotal: 5,
		},
		{
			name: "Success - Get all deadlines returns empty list",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:     false,
			expectedCount: 0,
			expectedTotal: 0,
		},
		{
			name: "Error - Repository FindList fails",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name: "Error - Repository Count fails",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(0), constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, total, err := svc.GetAllDeadlines(ctx, tt.filters)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
				assert.Equal(t, int64(0), total)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result, tt.expectedCount)
				assert.Equal(t, tt.expectedTotal, total)
			}
		})
	}
}

func TestUseCase_UpdateDeadlineByID(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()
	newName := "Updated Deadline Name"
	newColor := "#00FF00"
	newDesc := "Updated description"
	newType := "custom"
	newFreq := "annual"
	newTemplateID := uuid.New()
	monthsOfYear := []int{6}
	notifyDays := 10
	activeTrue := true
	activeFalse := false
	newDueDate := time.Now().Add(48 * time.Hour)

	// currentDeadline returns a baseline deadline used as the "existing state" for merge validation.
	currentDeadline := func() *deadline.Deadline {
		return &deadline.Deadline{
			ID:        deadlineID,
			Name:      "Existing Deadline",
			Type:      "regulatory",
			DueDate:   time.Now().Add(24 * time.Hour),
			Frequency: "monthly",
			Color:     "#FF5733",
			Active:    true,
			Status:    deadline.StatusPending,
		}
	}

	tests := []struct {
		name           string
		id             uuid.UUID
		input          *deadline.UpdateDeadlineInput
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult bool
	}{
		{
			name: "Success - Update deadline name",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Name: &newName,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:        deadlineID,
						Name:      newName,
						Type:      "regulatory",
						DueDate:   time.Now().Add(24 * time.Hour),
						Frequency: "monthly",
						Color:     "#FF5733",
						Active:    true,
						Status:    deadline.StatusPending,
					}, nil).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Activate deadline",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Active: &activeTrue,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Some Deadline",
						Active: true,
						Status: deadline.StatusPending,
					}, nil).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Deactivate deadline",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Active: &activeFalse,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Some Deadline",
						Active: false,
						Status: deadline.StatusPending,
					}, nil).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Update multiple fields",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Name:    &newName,
				Color:   &newColor,
				DueDate: &newDueDate,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:      deadlineID,
						Name:    newName,
						Color:   newColor,
						DueDate: newDueDate,
						Status:  deadline.StatusPending,
					}, nil).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Update all optional fields",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Name:             &newName,
				Description:      &newDesc,
				Type:             &newType,
				TemplateID:       &newTemplateID,
				DueDate:          &newDueDate,
				Frequency:        &newFreq,
				MonthsOfYear:     monthsOfYear,
				Active:           &activeTrue,
				NotifyDaysBefore: &notifyDays,
				Color:            &newColor,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)
				mockTemplateRepo := template.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				tmplFind := mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), newTemplateID).
					Return(&template.Template{
						ID:          newTemplateID,
						Description: "Updated Template",
					}, nil).After(first)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(tmplFind)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:               deadlineID,
						Name:             newName,
						Description:      newDesc,
						Type:             newType,
						TemplateID:       &newTemplateID,
						TemplateName:     "Updated Template",
						DueDate:          newDueDate,
						Frequency:        newFreq,
						MonthsOfYear:     monthsOfYear,
						Active:           true,
						NotifyDaysBefore: notifyDays,
						Color:            newColor,
						Status:           deadline.StatusPending,
					}, nil).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
					TemplateRepo: mockTemplateRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Error - Invalid type in update",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Type: func() *string { s := "invalid_type"; return &s }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInvalidDeadlineType.Error(),
		},
		{
			name: "Error - Invalid frequency in update",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Frequency: func() *string { s := "invalid_freq"; return &s }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInvalidDeadlineFrequency.Error(),
		},
		{
			name: "Error - monthsOfYear not applicable for monthly frequency in update",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Frequency:    func() *string { s := "monthly"; return &s }(),
				MonthsOfYear: monthsOfYear,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrMonthsOfYearNotApplicable.Error(),
		},
		{
			name: "Error - Invalid color in update",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Color: func() *string { s := "not-a-color"; return &s }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInvalidDeadlineColor.Error(),
		},
		{
			name: "Error - Duplicate key on update",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Type: func() *string { s := "regulatory"; return &s }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(mongo.CommandError{Code: 11000, Message: "duplicate key error"}).After(first)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: "deadline with the same name",
		},
		{
			name: "Error - Repository Update fails",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Name: &newName,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(constant.ErrInternalServer).After(first)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name: "Error - FindByID after update fails",
			id:   deadlineID,
			input: &deadline.UpdateDeadlineInput{
				Name: &newName,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDeadline(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(nil, constant.ErrEntityNotFound).After(update)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrEntityNotFound.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, err := svc.UpdateDeadlineByID(ctx, tt.id, tt.input)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.id, result.ID)

				if tt.input.Name != nil {
					assert.Equal(t, *tt.input.Name, result.Name)
				}

				if tt.input.Color != nil {
					assert.Equal(t, *tt.input.Color, result.Color)
				}
			}
		})
	}
}

func TestUseCase_DeleteDeadlineByID(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()

	tests := []struct {
		name        string
		id          uuid.UUID
		mockSetup   func(ctrl *gomock.Controller) *UseCase
		expectErr   bool
		errContains string
	}{
		{
			name: "Success - Soft delete deadline",
			id:   deadlineID,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr: false,
		},
		{
			name: "Error - Repository Delete fails",
			id:   deadlineID,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name: "Error - Deadline not found on delete",
			id:   deadlineID,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(constant.ErrEntityNotFound)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrEntityNotFound.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()
			err := svc.DeleteDeadlineByID(ctx, tt.id)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUseCase_DeliverDeadline(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()

	tests := []struct {
		name           string
		id             uuid.UUID
		input          *deadline.DeliverDeadlineInput
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult bool
	}{
		{
			name: "Success - Mark deadline as delivered",
			id:   deadlineID,
			input: &deadline.DeliverDeadlineInput{
				Delivered: func() *bool { b := true; return &b }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				// Update should set delivered_at to now
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					DoAndReturn(func(ctx context.Context, id uuid.UUID, fields *bson.M) error {
						// Verify that $set contains delivered_at with a non-nil time
						setFields, ok := (*fields)["$set"].(bson.M)
						require.True(t, ok)
						_, hasDeliveredAt := setFields["delivered_at"]
						assert.True(t, hasDeliveredAt, "delivered_at should be set when delivered=true")

						return nil
					})

				now := time.Now()
				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:          deadlineID,
						Name:        "Monthly Report",
						Status:      deadline.StatusDelivered,
						DeliveredAt: &now,
					}, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Success - Clear delivered status (delivered=false)",
			id:   deadlineID,
			input: &deadline.DeliverDeadlineInput{
				Delivered: func() *bool { b := false; return &b }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				// Update should set delivered_at to nil
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					DoAndReturn(func(ctx context.Context, id uuid.UUID, fields *bson.M) error {
						setFields, ok := (*fields)["$set"].(bson.M)
						require.True(t, ok)
						deliveredAt := setFields["delivered_at"]
						assert.Nil(t, deliveredAt, "delivered_at should be nil when delivered=false")

						return nil
					})

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Monthly Report",
						Status: deadline.StatusPending,
					}, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name: "Error - Repository Update fails on deliver",
			id:   deadlineID,
			input: &deadline.DeliverDeadlineInput{
				Delivered: func() *bool { b := true; return &b }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name: "Error - FindByID after deliver fails",
			id:   deadlineID,
			input: &deadline.DeliverDeadlineInput{
				Delivered: func() *bool { b := true; return &b }(),
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockDeadlineRepo := deadline.NewMockRepository(ctrl)

				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(nil, constant.ErrEntityNotFound)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					DeadlineRepo: mockDeadlineRepo,
				}
			},
			expectErr:   true,
			errContains: constant.ErrEntityNotFound.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			svc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, err := svc.DeliverDeadline(ctx, tt.id, tt.input)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, deadlineID, result.ID)

				if *tt.input.Delivered {
					assert.Equal(t, deadline.StatusDelivered, result.Status)
					assert.NotNil(t, result.DeliveredAt)
				} else {
					assert.NotEqual(t, deadline.StatusDelivered, result.Status)
					assert.Nil(t, result.DeliveredAt)
				}
			}
		})
	}
}

func TestComputeStatus(t *testing.T) {
	t.Parallel()

	now := time.Now()
	pastDate := now.Add(-24 * time.Hour)
	futureDate := now.Add(24 * time.Hour)

	tests := []struct {
		name        string
		dueDate     time.Time
		deliveredAt *time.Time
		expected    string
	}{
		{
			name:        "Success - Status is delivered when deliveredAt is set",
			dueDate:     futureDate,
			deliveredAt: &now,
			expected:    deadline.StatusDelivered,
		},
		{
			name:        "Success - Status is overdue when past due date and not delivered",
			dueDate:     pastDate,
			deliveredAt: nil,
			expected:    deadline.StatusOverdue,
		},
		{
			name:        "Success - Status is pending when future due date and not delivered",
			dueDate:     futureDate,
			deliveredAt: nil,
			expected:    deadline.StatusPending,
		},
		{
			name:        "Success - Status is delivered even when past due date",
			dueDate:     pastDate,
			deliveredAt: &now,
			expected:    deadline.StatusDelivered,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := deadline.ComputeStatus(tt.dueDate, tt.deliveredAt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNextDueDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dueDate      time.Time
		frequency    string
		monthsOfYear []int
		expected     time.Time
	}{
		{
			name:      "once - no advancement",
			dueDate:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			frequency: "once",
			expected:  time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "daily - next day",
			dueDate:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			frequency: "daily",
			expected:  time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "weekly - plus 7 days",
			dueDate:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			frequency: "weekly",
			expected:  time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "monthly - day 15 next month",
			dueDate:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			frequency: "monthly",
			expected:  time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "monthly - day 31 clamped to Feb 28",
			dueDate:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			frequency: "monthly",
			expected:  time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "semiannual - jan to jul",
			dueDate:      time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			frequency:    "semiannual",
			monthsOfYear: []int{1, 7},
			expected:     time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "semiannual - jul wraps to jan next year",
			dueDate:      time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			frequency:    "semiannual",
			monthsOfYear: []int{1, 7},
			expected:     time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "annual - apr to next apr",
			dueDate:      time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
			frequency:    "annual",
			monthsOfYear: []int{4},
			expected:     time.Date(2027, 4, 25, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := deadline.NextDueDate(tt.dueDate, tt.frequency, tt.monthsOfYear)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAdvanceRecurrence(t *testing.T) {
	t.Parallel()

	delivered := time.Date(2099, 1, 14, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		deadline          deadline.Deadline
		now               time.Time
		expectedDueDate   time.Time
		expectedDelivered bool
		expectedStatus    string
	}{
		{
			name: "monthly - advances to next month when delivered and past due",
			deadline: deadline.Deadline{
				DueDate:   time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
				Frequency: "monthly",

				DeliveredAt: &delivered,
				Status:      deadline.StatusDelivered,
			},
			now:               time.Date(2099, 2, 10, 0, 0, 0, 0, time.UTC),
			expectedDueDate:   time.Date(2099, 2, 15, 0, 0, 0, 0, time.UTC),
			expectedDelivered: false,
			expectedStatus:    deadline.StatusPending,
		},
		{
			name: "monthly - skips multiple months if user was away",
			deadline: deadline.Deadline{
				DueDate:   time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
				Frequency: "monthly",

				DeliveredAt: &delivered,
				Status:      deadline.StatusDelivered,
			},
			now:               time.Date(2099, 5, 1, 0, 0, 0, 0, time.UTC),
			expectedDueDate:   time.Date(2099, 5, 15, 0, 0, 0, 0, time.UTC),
			expectedDelivered: false,
			expectedStatus:    deadline.StatusPending,
		},
		{
			name: "once - never advances",
			deadline: deadline.Deadline{
				DueDate:     time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
				Frequency:   "once",
				DeliveredAt: &delivered,
				Status:      deadline.StatusDelivered,
			},
			now:               time.Date(2099, 5, 1, 0, 0, 0, 0, time.UTC),
			expectedDueDate:   time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
			expectedDelivered: true,
			expectedStatus:    deadline.StatusDelivered,
		},
		{
			name: "not delivered - does not advance even if past due",
			deadline: deadline.Deadline{
				DueDate:   time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
				Frequency: "monthly",

				Status: deadline.StatusOverdue,
			},
			now:               time.Date(2099, 2, 10, 0, 0, 0, 0, time.UTC),
			expectedDueDate:   time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC),
			expectedDelivered: false,
			expectedStatus:    deadline.StatusOverdue,
		},
		{
			name: "delivered but dueDate is still in the future - no advance",
			deadline: deadline.Deadline{
				DueDate:   time.Date(2099, 2, 15, 0, 0, 0, 0, time.UTC),
				Frequency: "monthly",

				DeliveredAt: &delivered,
				Status:      deadline.StatusDelivered,
			},
			now:               time.Date(2099, 2, 10, 0, 0, 0, 0, time.UTC),
			expectedDueDate:   time.Date(2099, 2, 15, 0, 0, 0, 0, time.UTC),
			expectedDelivered: true,
			expectedStatus:    deadline.StatusDelivered,
		},
		{
			name: "daily - advances to next day",
			deadline: deadline.Deadline{
				DueDate:     time.Now().Truncate(24 * time.Hour),
				Frequency:   "daily",
				DeliveredAt: &delivered,
				Status:      deadline.StatusDelivered,
			},
			now:               time.Now().Truncate(24 * time.Hour).Add(12 * time.Hour),
			expectedDueDate:   time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour),
			expectedDelivered: false,
			expectedStatus:    deadline.StatusPending,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := tt.deadline
			d.AdvanceRecurrence(tt.now)

			assert.Equal(t, tt.expectedDueDate, d.DueDate)
			assert.Equal(t, tt.expectedStatus, d.Status)

			if tt.expectedDelivered {
				assert.NotNil(t, d.DeliveredAt)
			} else {
				assert.Nil(t, d.DeliveredAt)
			}
		})
	}
}
