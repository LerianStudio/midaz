package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockAccountsService is a mock of AccountsService interface.
type MockAccountsService struct {
	ctrl     *gomock.Controller
	recorder *MockAccountsServiceMockRecorder
}

// MockAccountsServiceMockRecorder is the mock recorder for MockAccountsService.
type MockAccountsServiceMockRecorder struct {
	mock *MockAccountsService
}

// NewMockAccountsService creates a new mock instance.
func NewMockAccountsService(ctrl *gomock.Controller) *MockAccountsService {
	mock := &MockAccountsService{ctrl: ctrl}

	mock.recorder = &MockAccountsServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAccountsService) EXPECT() *MockAccountsServiceMockRecorder {
	return m.recorder
}

// ListAccounts mocks base method.
func (m *MockAccountsService) ListAccounts(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Account], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAccounts", ctx, organizationID, ledgerID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Account])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListAccounts indicates an expected call of ListAccounts.
func (mr *MockAccountsServiceMockRecorder) ListAccounts(ctx, organizationID, ledgerID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAccounts", reflect.TypeOf((*MockAccountsService)(nil).ListAccounts), ctx, organizationID, ledgerID, opts)
}

// GetAccount mocks base method.
func (m *MockAccountsService) GetAccount(ctx context.Context, organizationID, ledgerID, id string) (*models.Account, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAccount", ctx, organizationID, ledgerID, id)
	ret0, _ := ret[0].(*models.Account)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAccount indicates an expected call of GetAccount.
func (mr *MockAccountsServiceMockRecorder) GetAccount(ctx, organizationID, ledgerID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAccount", reflect.TypeOf((*MockAccountsService)(nil).GetAccount), ctx, organizationID, ledgerID, id)
}

// GetAccountByAlias mocks base method.
func (m *MockAccountsService) GetAccountByAlias(ctx context.Context, organizationID, ledgerID, alias string) (*models.Account, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAccountByAlias", ctx, organizationID, ledgerID, alias)
	ret0, _ := ret[0].(*models.Account)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAccountByAlias indicates an expected call of GetAccountByAlias.
func (mr *MockAccountsServiceMockRecorder) GetAccountByAlias(ctx, organizationID, ledgerID, alias interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAccountByAlias", reflect.TypeOf((*MockAccountsService)(nil).GetAccountByAlias), ctx, organizationID, ledgerID, alias)
}

// CreateAccount mocks base method.
func (m *MockAccountsService) CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateAccount", ctx, organizationID, ledgerID, input)
	ret0, _ := ret[0].(*models.Account)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateAccount indicates an expected call of CreateAccount.
func (mr *MockAccountsServiceMockRecorder) CreateAccount(ctx, organizationID, ledgerID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateAccount", reflect.TypeOf((*MockAccountsService)(nil).CreateAccount), ctx, organizationID, ledgerID, input)
}

// UpdateAccount mocks base method.
func (m *MockAccountsService) UpdateAccount(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAccountInput) (*models.Account, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateAccount", ctx, organizationID, ledgerID, id, input)
	ret0, _ := ret[0].(*models.Account)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateAccount indicates an expected call of UpdateAccount.
func (mr *MockAccountsServiceMockRecorder) UpdateAccount(ctx, organizationID, ledgerID, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateAccount", reflect.TypeOf((*MockAccountsService)(nil).UpdateAccount), ctx, organizationID, ledgerID, id, input)
}

// DeleteAccount mocks base method.
func (m *MockAccountsService) DeleteAccount(ctx context.Context, organizationID, ledgerID, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteAccount", ctx, organizationID, ledgerID, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteAccount indicates an expected call of DeleteAccount.
func (mr *MockAccountsServiceMockRecorder) DeleteAccount(ctx, organizationID, ledgerID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteAccount", reflect.TypeOf((*MockAccountsService)(nil).DeleteAccount), ctx, organizationID, ledgerID, id)
}

// GetBalance mocks base method.
func (m *MockAccountsService) GetBalance(ctx context.Context, organizationID, ledgerID, accountID string) (*models.Balance, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBalance", ctx, organizationID, ledgerID, accountID)
	ret0, _ := ret[0].(*models.Balance)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBalance indicates an expected call of GetBalance.
func (mr *MockAccountsServiceMockRecorder) GetBalance(ctx, organizationID, ledgerID, accountID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBalance", reflect.TypeOf((*MockAccountsService)(nil).GetBalance), ctx, organizationID, ledgerID, accountID)
}
