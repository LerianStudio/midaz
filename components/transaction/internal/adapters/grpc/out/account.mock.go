// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/LerianStudio/midaz/components/transaction/internal/adapters/grpc/out (interfaces: Repository)
//
// Generated by this command:
//
//	mockgen --destination=account.mock.go --package=out . Repository
//

// Package out is a generated GoMock package.
package out

import (
	context "context"
	gomock "go.uber.org/mock/gomock"
	reflect "reflect"

	account "github.com/LerianStudio/midaz/pkg/mgrpc/account"

	uuid "github.com/google/uuid"
)

// MockRepository is a mock of Repository interface.
type MockRepository struct {
	ctrl     *gomock.Controller
	recorder *MockRepositoryMockRecorder
}

// MockRepositoryMockRecorder is the mock recorder for MockRepository.
type MockRepositoryMockRecorder struct {
	mock *MockRepository
}

// NewMockRepository creates a new mock instance.
func NewMockRepository(ctrl *gomock.Controller) *MockRepository {
	mock := &MockRepository{ctrl: ctrl}
	mock.recorder = &MockRepositoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRepository) EXPECT() *MockRepositoryMockRecorder {
	return m.recorder
}

// GetAccountsByAlias mocks base method.
func (m *MockRepository) GetAccountsByAlias(arg0 context.Context, arg1 string, arg2, arg3 uuid.UUID, arg4 []string) (*account.AccountsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAccountsByAlias", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(*account.AccountsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAccountsByAlias indicates an expected call of GetAccountsByAlias.
func (mr *MockRepositoryMockRecorder) GetAccountsByAlias(arg0, arg1, arg2, arg3, arg4 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAccountsByAlias", reflect.TypeOf((*MockRepository)(nil).GetAccountsByAlias), arg0, arg1, arg2, arg3, arg4)
}

// GetAccountsByIds mocks base method.
func (m *MockRepository) GetAccountsByIds(arg0 context.Context, arg1 string, arg2, arg3 uuid.UUID, arg4 []string) (*account.AccountsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAccountsByIds", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(*account.AccountsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAccountsByIds indicates an expected call of GetAccountsByIds.
func (mr *MockRepositoryMockRecorder) GetAccountsByIds(arg0, arg1, arg2, arg3, arg4 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAccountsByIds", reflect.TypeOf((*MockRepository)(nil).GetAccountsByIds), arg0, arg1, arg2, arg3, arg4)
}

// UpdateAccounts mocks base method.
func (m *MockRepository) UpdateAccounts(arg0 context.Context, arg1 string, arg2, arg3 uuid.UUID, arg4 []*account.Account) (*account.AccountsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateAccounts", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(*account.AccountsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateAccounts indicates an expected call of UpdateAccounts.
func (mr *MockRepositoryMockRecorder) UpdateAccounts(arg0, arg1, arg2, arg3, arg4 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateAccounts", reflect.TypeOf((*MockRepository)(nil).UpdateAccounts), arg0, arg1, arg2, arg3, arg4)
}