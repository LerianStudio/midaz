package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockOperationsService is a mock of OperationsService interface.
type MockOperationsService struct {
	ctrl     *gomock.Controller
	recorder *MockOperationsServiceMockRecorder
}

// MockOperationsServiceMockRecorder is the mock recorder for MockOperationsService.
type MockOperationsServiceMockRecorder struct {
	mock *MockOperationsService
}

// NewMockOperationsService creates a new mock instance.
func NewMockOperationsService(ctrl *gomock.Controller) *MockOperationsService {
	mock := &MockOperationsService{ctrl: ctrl}
	mock.recorder = &MockOperationsServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOperationsService) EXPECT() *MockOperationsServiceMockRecorder {
	return m.recorder
}

// ListOperations mocks base method.
func (m *MockOperationsService) ListOperations(ctx context.Context, orgID, ledgerID, accountID string, opts *models.ListOptions) (*models.ListResponse[models.Operation], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListOperations", ctx, orgID, ledgerID, accountID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Operation])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListOperations indicates an expected call of ListOperations.
func (mr *MockOperationsServiceMockRecorder) ListOperations(ctx, orgID, ledgerID, accountID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListOperations", reflect.TypeOf((*MockOperationsService)(nil).ListOperations), ctx, orgID, ledgerID, accountID, opts)
}

// GetOperation mocks base method.
func (m *MockOperationsService) GetOperation(ctx context.Context, orgID, ledgerID, accountID, operationID string) (*models.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOperation", ctx, orgID, ledgerID, accountID, operationID)
	ret0, _ := ret[0].(*models.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOperation indicates an expected call of GetOperation.
func (mr *MockOperationsServiceMockRecorder) GetOperation(ctx, orgID, ledgerID, accountID, operationID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOperation", reflect.TypeOf((*MockOperationsService)(nil).GetOperation), ctx, orgID, ledgerID, accountID, operationID)
}

// UpdateOperation mocks base method.
func (m *MockOperationsService) UpdateOperation(ctx context.Context, orgID, ledgerID, transactionID, operationID string, input any) (*models.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOperation", ctx, orgID, ledgerID, transactionID, operationID, input)
	ret0, _ := ret[0].(*models.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateOperation indicates an expected call of UpdateOperation.
func (mr *MockOperationsServiceMockRecorder) UpdateOperation(ctx, orgID, ledgerID, transactionID, operationID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOperation", reflect.TypeOf((*MockOperationsService)(nil).UpdateOperation), ctx, orgID, ledgerID, transactionID, operationID, input)
}
