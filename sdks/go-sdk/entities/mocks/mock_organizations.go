package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockOrganizationsService is a mock of OrganizationsService interface.
type MockOrganizationsService struct {
	ctrl     *gomock.Controller
	recorder *MockOrganizationsServiceMockRecorder
}

// MockOrganizationsServiceMockRecorder is the mock recorder for MockOrganizationsService.
type MockOrganizationsServiceMockRecorder struct {
	mock *MockOrganizationsService
}

// NewMockOrganizationsService creates a new mock instance.
func NewMockOrganizationsService(ctrl *gomock.Controller) *MockOrganizationsService {
	mock := &MockOrganizationsService{ctrl: ctrl}

	mock.recorder = &MockOrganizationsServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOrganizationsService) EXPECT() *MockOrganizationsServiceMockRecorder {
	return m.recorder
}

// ListOrganizations mocks base method.
func (m *MockOrganizationsService) ListOrganizations(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.Organization], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListOrganizations", ctx, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Organization])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListOrganizations indicates an expected call of ListOrganizations.
func (mr *MockOrganizationsServiceMockRecorder) ListOrganizations(ctx, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListOrganizations", reflect.TypeOf((*MockOrganizationsService)(nil).ListOrganizations), ctx, opts)
}

// GetOrganization mocks base method.
func (m *MockOrganizationsService) GetOrganization(ctx context.Context, id string) (*models.Organization, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrganization", ctx, id)
	ret0, _ := ret[0].(*models.Organization)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrganization indicates an expected call of GetOrganization.
func (mr *MockOrganizationsServiceMockRecorder) GetOrganization(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrganization", reflect.TypeOf((*MockOrganizationsService)(nil).GetOrganization), ctx, id)
}

// CreateOrganization mocks base method.
func (m *MockOrganizationsService) CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateOrganization", ctx, input)
	ret0, _ := ret[0].(*models.Organization)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateOrganization indicates an expected call of CreateOrganization.
func (mr *MockOrganizationsServiceMockRecorder) CreateOrganization(ctx, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateOrganization", reflect.TypeOf((*MockOrganizationsService)(nil).CreateOrganization), ctx, input)
}

// UpdateOrganization mocks base method.
func (m *MockOrganizationsService) UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOrganization", ctx, id, input)
	ret0, _ := ret[0].(*models.Organization)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateOrganization indicates an expected call of UpdateOrganization.
func (mr *MockOrganizationsServiceMockRecorder) UpdateOrganization(ctx, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOrganization", reflect.TypeOf((*MockOrganizationsService)(nil).UpdateOrganization), ctx, id, input)
}

// DeleteOrganization mocks base method.
func (m *MockOrganizationsService) DeleteOrganization(ctx context.Context, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteOrganization", ctx, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteOrganization indicates an expected call of DeleteOrganization.
func (mr *MockOrganizationsServiceMockRecorder) DeleteOrganization(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteOrganization", reflect.TypeOf((*MockOrganizationsService)(nil).DeleteOrganization), ctx, id)
}
