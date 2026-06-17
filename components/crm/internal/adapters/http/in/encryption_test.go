// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEncryption_Provision(t *testing.T) {
	tests := []struct {
		name           string
		organizationID string
		tenantID       string
		jsonBody       string
		setupMocks     func(mockService *MockProvisioningService, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "success returns 201 with provision result",
			organizationID: uuid.New().String(),
			jsonBody: `{
				"actor": "admin@example.com",
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					Provision(gomock.Any(), gomock.Cond(func(x any) bool {
						req, ok := x.(encryption.ProvisionInput)
						if !ok {
							return false
						}
						return req.OrganizationID == orgID &&
							req.Actor == "admin@example.com" &&
							req.Reason == "Initial encryption setup"
					})).
					Return(encryption.ProvisionResult{
						OrganizationID:   orgID,
						KEKPath:          "transit/keys/org-" + orgID,
						AEADPrimaryKeyID: 123456,
						PRFPrimaryKeyID:  789012,
						RegistryStatus:   mmodel.RegistryStatusActive,
					}, nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result mmodel.ProvisionEncryptionResponse
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.NotEmpty(t, result.OrganizationID, "response should contain organization_id")
				assert.NotEmpty(t, result.KEKPath, "response should contain kek_path")
				assert.Equal(t, string(mmodel.RegistryStatusActive), result.Status)
			},
		},
		{
			name:           "missing actor returns 400",
			organizationID: uuid.New().String(),
			jsonBody: `{
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				// No mock expectations - validation should fail before reaching service
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name:           "missing reason returns 400",
			organizationID: uuid.New().String(),
			jsonBody: `{
				"actor": "admin@example.com"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				// No mock expectations - validation should fail before reaching service
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name:           "organization already provisioned returns 409",
			organizationID: uuid.New().String(),
			jsonBody: `{
				"actor": "admin@example.com",
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					Provision(gomock.Any(), gomock.Any()).
					Return(encryption.ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrRegistryAlreadyExists, encryption.EntityOrganizationEncryption)).
					Times(1)
			},
			expectedStatus: 409,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
			},
		},
		{
			name:           "provisioning failed returns 500",
			organizationID: uuid.New().String(),
			jsonBody: `{
				"actor": "admin@example.com",
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					Provision(gomock.Any(), gomock.Any()).
					Return(encryption.ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, encryption.EntityOrganizationEncryption)).
					Times(1)
			},
			expectedStatus: 500,
			validateBody:   nil, // Error format handled by http.WithError middleware
		},
		{
			// Multi-tenant mode: the tenant middleware populated a real, non-empty
			// tenant id literally equal to "default" from the JWT. This collides
			// with the single-tenant flat-base sentinel, so it MUST be rejected
			// before reaching the provisioning service (no Provision call).
			name:           "real tenant named default returns 422",
			organizationID: uuid.New().String(),
			tenantID:       "default",
			jsonBody: `{
				"actor": "admin@example.com",
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				// No mock expectations: rejection happens before the service call.
			},
			expectedStatus: 422,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Equal(t, constant.ErrReservedTenantID.Error(), errResp["code"],
					"reserved tenant id rejection should map to ErrReservedTenantID code")
			},
		},
		{
			// Single-tenant sentinel path: no tenant middleware ran, so the
			// context carries no tenant id (empty). The handler substitutes the
			// "default" flat-base sentinel and provisioning proceeds normally.
			name:           "single-tenant default sentinel still provisions",
			organizationID: uuid.New().String(),
			tenantID:       "", // empty context => single-tenant sentinel path
			jsonBody: `{
				"actor": "admin@example.com",
				"reason": "Initial encryption setup"
			}`,
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					Provision(gomock.Any(), gomock.Cond(func(x any) bool {
						req, ok := x.(encryption.ProvisionInput)
						if !ok {
							return false
						}
						return req.OrganizationID == orgID && req.TenantID == "default"
					})).
					Return(encryption.ProvisionResult{
						OrganizationID: orgID,
						KEKPath:        "transit/keys/org-" + orgID,
						RegistryStatus: mmodel.RegistryStatusActive,
					}, nil).
					Times(1)
			},
			expectedStatus: 201,
			validateBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockService := NewMockProvisioningService(ctrl)
			tt.setupMocks(mockService, tt.organizationID)

			handler := &EncryptionHandler{
				ProvisioningService: mockService,
			}

			tenantID := tt.tenantID

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/encryption/provision",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", uuid.MustParse(tt.organizationID))
					// Simulate the tenant middleware: in multi-tenant mode it
					// stores a non-empty tenant id; in single-tenant mode it does
					// not run, so the context carries no tenant id.
					if tenantID != "" {
						c.SetUserContext(tmcore.ContextWithTenantID(c.UserContext(), tenantID))
					}
					return c.Next()
				},
				http.WithBody(new(mmodel.ProvisionEncryptionInput), handler.Provision),
			)

			req := httptest.NewRequest("POST", "/v1/organizations/"+tt.organizationID+"/encryption/provision", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestEncryptionHandler_GetProvisioningStatus(t *testing.T) {
	tests := []struct {
		name           string
		organizationID string
		setupMocks     func(mockService *MockProvisioningService, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "success with active status returns 200",
			organizationID: uuid.New().String(),
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				status := mmodel.RegistryStatusActive
				mockService.EXPECT().
					GetProvisioningStatus(gomock.Any(), orgID).
					Return(&status, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result mmodel.ProvisioningStatusResponse
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.NotEmpty(t, result.OrganizationID, "response should contain organization_id")
				assert.Equal(t, string(mmodel.RegistryStatusActive), result.Status)
				assert.True(t, result.Provisioned, "provisioned should be true")
			},
		},
		{
			name:           "not provisioned returns 200 with provisioned false",
			organizationID: uuid.New().String(),
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					GetProvisioningStatus(gomock.Any(), orgID).
					Return(nil, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result mmodel.ProvisioningStatusResponse
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.NotEmpty(t, result.OrganizationID, "response should contain organization_id")
				assert.Empty(t, result.Status, "status should be empty for not provisioned")
				assert.False(t, result.Provisioned, "provisioned should be false")
			},
		},
		{
			name:           "service error returns 500",
			organizationID: uuid.New().String(),
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					GetProvisioningStatus(gomock.Any(), orgID).
					Return(nil, errors.New("database error")).
					Times(1)
			},
			expectedStatus: 500,
			validateBody:   nil, // Error format handled by http.WithError middleware
		},
		{
			name:           "context cancelled returns 500",
			organizationID: uuid.New().String(),
			setupMocks: func(mockService *MockProvisioningService, orgID string) {
				mockService.EXPECT().
					GetProvisioningStatus(gomock.Any(), orgID).
					Return(nil, context.Canceled).
					Times(1)
			},
			expectedStatus: 500,
			validateBody:   nil, // Error format handled by http.WithError middleware
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockService := NewMockProvisioningService(ctrl)
			tt.setupMocks(mockService, tt.organizationID)

			handler := &EncryptionHandler{
				ProvisioningService: mockService,
			}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/encryption/status",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", uuid.MustParse(tt.organizationID))
					return c.Next()
				},
				handler.GetProvisioningStatus,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+tt.organizationID+"/encryption/status", nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

// MockProvisioningService is a mock implementation of the ProvisioningService interface.
// Generated by gomock but defined here for TDD-RED phase to show expected interface.
type MockProvisioningService struct {
	ctrl     *gomock.Controller
	recorder *MockProvisioningServiceRecorder
}

type MockProvisioningServiceRecorder struct {
	mock *MockProvisioningService
}

func NewMockProvisioningService(ctrl *gomock.Controller) *MockProvisioningService {
	mock := &MockProvisioningService{ctrl: ctrl}
	mock.recorder = &MockProvisioningServiceRecorder{mock}
	return mock
}

func (m *MockProvisioningService) EXPECT() *MockProvisioningServiceRecorder {
	return m.recorder
}

func (m *MockProvisioningService) Provision(ctx context.Context, req encryption.ProvisionInput) (encryption.ProvisionResult, error) {
	ret := m.ctrl.Call(m, "Provision", ctx, req)
	return ret[0].(encryption.ProvisionResult), errOrNil(ret[1])
}

func (mr *MockProvisioningServiceRecorder) Provision(ctx, req any) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Provision", reflect.TypeOf((*MockProvisioningService)(nil).Provision), ctx, req)
}

func (m *MockProvisioningService) GetProvisioningStatus(ctx context.Context, organizationID string) (*mmodel.RegistryStatus, error) {
	ret := m.ctrl.Call(m, "GetProvisioningStatus", ctx, organizationID)
	status, _ := ret[0].(*mmodel.RegistryStatus)
	return status, errOrNil(ret[1])
}

func (mr *MockProvisioningServiceRecorder) GetProvisioningStatus(ctx, organizationID any) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetProvisioningStatus", reflect.TypeOf((*MockProvisioningService)(nil).GetProvisioningStatus), ctx, organizationID)
}

func (m *MockProvisioningService) IsProvisioned(ctx context.Context, organizationID string) (bool, error) {
	ret := m.ctrl.Call(m, "IsProvisioned", ctx, organizationID)
	return ret[0].(bool), errOrNil(ret[1])
}

func (mr *MockProvisioningServiceRecorder) IsProvisioned(ctx, organizationID any) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsProvisioned", reflect.TypeOf((*MockProvisioningService)(nil).IsProvisioned), ctx, organizationID)
}

func (m *MockProvisioningService) IsActive(ctx context.Context, organizationID string) (bool, error) {
	ret := m.ctrl.Call(m, "IsActive", ctx, organizationID)
	return ret[0].(bool), errOrNil(ret[1])
}

func (mr *MockProvisioningServiceRecorder) IsActive(ctx, organizationID any) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsActive", reflect.TypeOf((*MockProvisioningService)(nil).IsActive), ctx, organizationID)
}

func errOrNil(v any) error {
	if v == nil {
		return nil
	}
	return v.(error)
}
