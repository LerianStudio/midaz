package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockLedgersService is a mock of LedgersService interface.
type MockLedgersService struct {
	ctrl     *gomock.Controller
	recorder *MockLedgersServiceMockRecorder
}

// MockLedgersServiceMockRecorder is the mock recorder for MockLedgersService.
type MockLedgersServiceMockRecorder struct {
	mock *MockLedgersService
}

// NewMockLedgersService creates a new mock instance.
func NewMockLedgersService(ctrl *gomock.Controller) *MockLedgersService {
	mock := &MockLedgersService{ctrl: ctrl}

	mock.recorder = &MockLedgersServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLedgersService) EXPECT() *MockLedgersServiceMockRecorder {
	return m.recorder
}

// ListLedgers mocks base method.
func (m *MockLedgersService) ListLedgers(ctx context.Context, organizationID string, opts *models.ListOptions) (*models.ListResponse[models.Ledger], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListLedgers", ctx, organizationID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Ledger])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListLedgers indicates an expected call of ListLedgers.
func (mr *MockLedgersServiceMockRecorder) ListLedgers(ctx, organizationID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListLedgers", reflect.TypeOf((*MockLedgersService)(nil).ListLedgers), ctx, organizationID, opts)
}

// GetLedger mocks base method.
func (m *MockLedgersService) GetLedger(ctx context.Context, organizationID, id string) (*models.Ledger, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLedger", ctx, organizationID, id)
	ret0, _ := ret[0].(*models.Ledger)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetLedger indicates an expected call of GetLedger.
func (mr *MockLedgersServiceMockRecorder) GetLedger(ctx, organizationID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLedger", reflect.TypeOf((*MockLedgersService)(nil).GetLedger), ctx, organizationID, id)
}

// CreateLedger mocks base method.
func (m *MockLedgersService) CreateLedger(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateLedger", ctx, organizationID, input)
	ret0, _ := ret[0].(*models.Ledger)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateLedger indicates an expected call of CreateLedger.
func (mr *MockLedgersServiceMockRecorder) CreateLedger(ctx, organizationID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateLedger", reflect.TypeOf((*MockLedgersService)(nil).CreateLedger), ctx, organizationID, input)
}

// UpdateLedger mocks base method.
func (m *MockLedgersService) UpdateLedger(ctx context.Context, organizationID, id string, input *models.UpdateLedgerInput) (*models.Ledger, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateLedger", ctx, organizationID, id, input)
	ret0, _ := ret[0].(*models.Ledger)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateLedger indicates an expected call of UpdateLedger.
func (mr *MockLedgersServiceMockRecorder) UpdateLedger(ctx, organizationID, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateLedger", reflect.TypeOf((*MockLedgersService)(nil).UpdateLedger), ctx, organizationID, id, input)
}

// DeleteLedger mocks base method.
func (m *MockLedgersService) DeleteLedger(ctx context.Context, organizationID, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteLedger", ctx, organizationID, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteLedger indicates an expected call of DeleteLedger.
func (mr *MockLedgersServiceMockRecorder) DeleteLedger(ctx, organizationID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteLedger", reflect.TypeOf((*MockLedgersService)(nil).DeleteLedger), ctx, organizationID, id)
}
