package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockBalancesService is a mock of BalancesService interface.
type MockBalancesService struct {
	ctrl     *gomock.Controller
	recorder *MockBalancesServiceMockRecorder
}

// MockBalancesServiceMockRecorder is the mock recorder for MockBalancesService.
type MockBalancesServiceMockRecorder struct {
	mock *MockBalancesService
}

// NewMockBalancesService creates a new mock instance.
func NewMockBalancesService(ctrl *gomock.Controller) *MockBalancesService {
	mock := &MockBalancesService{ctrl: ctrl}

	mock.recorder = &MockBalancesServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBalancesService) EXPECT() *MockBalancesServiceMockRecorder {
	return m.recorder
}

// ListBalances mocks base method.
func (m *MockBalancesService) ListBalances(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListBalances", ctx, orgID, ledgerID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Balance])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListBalances indicates an expected call of ListBalances.
func (mr *MockBalancesServiceMockRecorder) ListBalances(ctx, orgID, ledgerID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListBalances", reflect.TypeOf((*MockBalancesService)(nil).ListBalances), ctx, orgID, ledgerID, opts)
}

// ListAccountBalances mocks base method.
func (m *MockBalancesService) ListAccountBalances(ctx context.Context, orgID, ledgerID, accountID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAccountBalances", ctx, orgID, ledgerID, accountID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Balance])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListAccountBalances indicates an expected call of ListAccountBalances.
func (mr *MockBalancesServiceMockRecorder) ListAccountBalances(ctx, orgID, ledgerID, accountID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAccountBalances", reflect.TypeOf((*MockBalancesService)(nil).ListAccountBalances), ctx, orgID, ledgerID, accountID, opts)
}

// GetBalance mocks base method.
func (m *MockBalancesService) GetBalance(ctx context.Context, orgID, ledgerID, balanceID string) (*models.Balance, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBalance", ctx, orgID, ledgerID, balanceID)
	ret0, _ := ret[0].(*models.Balance)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBalance indicates an expected call of GetBalance.
func (mr *MockBalancesServiceMockRecorder) GetBalance(ctx, orgID, ledgerID, balanceID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBalance", reflect.TypeOf((*MockBalancesService)(nil).GetBalance), ctx, orgID, ledgerID, balanceID)
}

// UpdateBalance mocks base method.
func (m *MockBalancesService) UpdateBalance(ctx context.Context, orgID, ledgerID, balanceID string, input *models.UpdateBalanceInput) (*models.Balance, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateBalance", ctx, orgID, ledgerID, balanceID, input)
	ret0, _ := ret[0].(*models.Balance)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateBalance indicates an expected call of UpdateBalance.
func (mr *MockBalancesServiceMockRecorder) UpdateBalance(ctx, orgID, ledgerID, balanceID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateBalance", reflect.TypeOf((*MockBalancesService)(nil).UpdateBalance), ctx, orgID, ledgerID, balanceID, input)
}

// DeleteBalance mocks base method.
func (m *MockBalancesService) DeleteBalance(ctx context.Context, orgID, ledgerID, balanceID string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteBalance", ctx, orgID, ledgerID, balanceID)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteBalance indicates an expected call of DeleteBalance.
func (mr *MockBalancesServiceMockRecorder) DeleteBalance(ctx, orgID, ledgerID, balanceID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteBalance", reflect.TypeOf((*MockBalancesService)(nil).DeleteBalance), ctx, orgID, ledgerID, balanceID)
}
