package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockTransactionsService is a mock of TransactionsService interface.
type MockTransactionsService struct {
	ctrl     *gomock.Controller
	recorder *MockTransactionsServiceMockRecorder
}

// MockTransactionsServiceMockRecorder is the mock recorder for MockTransactionsService.
type MockTransactionsServiceMockRecorder struct {
	mock *MockTransactionsService
}

// NewMockTransactionsService creates a new mock instance.
func NewMockTransactionsService(ctrl *gomock.Controller) *MockTransactionsService {
	mock := &MockTransactionsService{ctrl: ctrl}

	mock.recorder = &MockTransactionsServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTransactionsService) EXPECT() *MockTransactionsServiceMockRecorder {
	return m.recorder
}

// CreateTransaction mocks base method.
func (m *MockTransactionsService) CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.CreateTransactionInput) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateTransaction", ctx, orgID, ledgerID, input)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateTransaction indicates an expected call of CreateTransaction.
func (mr *MockTransactionsServiceMockRecorder) CreateTransaction(ctx, orgID, ledgerID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTransaction", reflect.TypeOf((*MockTransactionsService)(nil).CreateTransaction), ctx, orgID, ledgerID, input)
}

// CreateTransactionWithDSL mocks base method.
func (m *MockTransactionsService) CreateTransactionWithDSL(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateTransactionWithDSL", ctx, orgID, ledgerID, input)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateTransactionWithDSL indicates an expected call of CreateTransactionWithDSL.
func (mr *MockTransactionsServiceMockRecorder) CreateTransactionWithDSL(ctx, orgID, ledgerID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTransactionWithDSL", reflect.TypeOf((*MockTransactionsService)(nil).CreateTransactionWithDSL), ctx, orgID, ledgerID, input)
}

// CreateTransactionWithDSLFile mocks base method.
func (m *MockTransactionsService) CreateTransactionWithDSLFile(ctx context.Context, orgID, ledgerID string, dslContent []byte) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateTransactionWithDSLFile", ctx, orgID, ledgerID, dslContent)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateTransactionWithDSLFile indicates an expected call of CreateTransactionWithDSLFile.
func (mr *MockTransactionsServiceMockRecorder) CreateTransactionWithDSLFile(ctx, orgID, ledgerID, dslContent interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTransactionWithDSLFile", reflect.TypeOf((*MockTransactionsService)(nil).CreateTransactionWithDSLFile), ctx, orgID, ledgerID, dslContent)
}

// GetTransaction mocks base method.
func (m *MockTransactionsService) GetTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransaction", ctx, orgID, ledgerID, transactionID)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTransaction indicates an expected call of GetTransaction.
func (mr *MockTransactionsServiceMockRecorder) GetTransaction(ctx, orgID, ledgerID, transactionID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransaction", reflect.TypeOf((*MockTransactionsService)(nil).GetTransaction), ctx, orgID, ledgerID, transactionID)
}

// ListTransactions mocks base method.
func (m *MockTransactionsService) ListTransactions(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListTransactions", ctx, orgID, ledgerID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Transaction])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListTransactions indicates an expected call of ListTransactions.
func (mr *MockTransactionsServiceMockRecorder) ListTransactions(ctx, orgID, ledgerID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListTransactions", reflect.TypeOf((*MockTransactionsService)(nil).ListTransactions), ctx, orgID, ledgerID, opts)
}

// UpdateTransaction mocks base method.
func (m *MockTransactionsService) UpdateTransaction(ctx context.Context, orgID, ledgerID, transactionID string, input any) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateTransaction", ctx, orgID, ledgerID, transactionID, input)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateTransaction indicates an expected call of UpdateTransaction.
func (mr *MockTransactionsServiceMockRecorder) UpdateTransaction(ctx, orgID, ledgerID, transactionID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateTransaction", reflect.TypeOf((*MockTransactionsService)(nil).UpdateTransaction), ctx, orgID, ledgerID, transactionID, input)
}

// CommitTransaction mocks base method.
func (m *MockTransactionsService) CommitTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CommitTransaction", ctx, orgID, ledgerID, transactionID)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CommitTransaction indicates an expected call of CommitTransaction.
func (mr *MockTransactionsServiceMockRecorder) CommitTransaction(ctx, orgID, ledgerID, transactionID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CommitTransaction", reflect.TypeOf((*MockTransactionsService)(nil).CommitTransaction), ctx, orgID, ledgerID, transactionID)
}

// CommitTransactionWithExternalID mocks base method.
func (m *MockTransactionsService) CommitTransactionWithExternalID(ctx context.Context, orgID, ledgerID, externalID string) (*models.Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CommitTransactionWithExternalID", ctx, orgID, ledgerID, externalID)
	ret0, _ := ret[0].(*models.Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CommitTransactionWithExternalID indicates an expected call of CommitTransactionWithExternalID.
func (mr *MockTransactionsServiceMockRecorder) CommitTransactionWithExternalID(ctx, orgID, ledgerID, externalID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CommitTransactionWithExternalID", reflect.TypeOf((*MockTransactionsService)(nil).CommitTransactionWithExternalID), ctx, orgID, ledgerID, externalID)
}
