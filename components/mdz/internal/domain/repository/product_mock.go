// Code generated by MockGen. DO NOT EDIT.
// Source: /Users/maxwelbm/Workspace/midaz/components/mdz/internal/domain/repository/product.go
//
// Generated by this command:
//
//	mockgen -source=/Users/maxwelbm/Workspace/midaz/components/mdz/internal/domain/repository/product.go -destination=/Users/maxwelbm/Workspace/midaz/components/mdz/internal/domain/repository/product_mock.go -package repository
//

// Package repository is a generated GoMock package.
package repository

import (
	reflect "reflect"

	mmodel "github.com/LerianStudio/midaz/common/mmodel"
	gomock "go.uber.org/mock/gomock"
)

// MockProduct is a mock of Product interface.
type MockProduct struct {
	ctrl     *gomock.Controller
	recorder *MockProductMockRecorder
	isgomock struct{}
}

// MockProductMockRecorder is the mock recorder for MockProduct.
type MockProductMockRecorder struct {
	mock *MockProduct
}

// NewMockProduct creates a new mock instance.
func NewMockProduct(ctrl *gomock.Controller) *MockProduct {
	mock := &MockProduct{ctrl: ctrl}
	mock.recorder = &MockProductMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockProduct) EXPECT() *MockProductMockRecorder {
	return m.recorder
}

// Create mocks base method.
func (m *MockProduct) Create(organizationID, ledgerID string, inp mmodel.CreateProductInput) (*mmodel.Product, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", organizationID, ledgerID, inp)
	ret0, _ := ret[0].(*mmodel.Product)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Create indicates an expected call of Create.
func (mr *MockProductMockRecorder) Create(organizationID, ledgerID, inp any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockProduct)(nil).Create), organizationID, ledgerID, inp)
}

// Delete mocks base method.
func (m *MockProduct) Delete(organizationID, ledgerID, productID string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", organizationID, ledgerID, productID)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockProductMockRecorder) Delete(organizationID, ledgerID, productID any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockProduct)(nil).Delete), organizationID, ledgerID, productID)
}

// Get mocks base method.
func (m *MockProduct) Get(organizationID, ledgerID string, limit, page int) (*mmodel.Products, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", organizationID, ledgerID, limit, page)
	ret0, _ := ret[0].(*mmodel.Products)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockProductMockRecorder) Get(organizationID, ledgerID, limit, page any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockProduct)(nil).Get), organizationID, ledgerID, limit, page)
}

// GetByID mocks base method.
func (m *MockProduct) GetByID(organizationID, ledgerID, productID string) (*mmodel.Product, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByID", organizationID, ledgerID, productID)
	ret0, _ := ret[0].(*mmodel.Product)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByID indicates an expected call of GetByID.
func (mr *MockProductMockRecorder) GetByID(organizationID, ledgerID, productID any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByID", reflect.TypeOf((*MockProduct)(nil).GetByID), organizationID, ledgerID, productID)
}

// Update mocks base method.
func (m *MockProduct) Update(organizationID, ledgerID, productID string, inp mmodel.UpdateProductInput) (*mmodel.Product, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", organizationID, ledgerID, productID, inp)
	ret0, _ := ret[0].(*mmodel.Product)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Update indicates an expected call of Update.
func (mr *MockProductMockRecorder) Update(organizationID, ledgerID, productID, inp any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockProduct)(nil).Update), organizationID, ledgerID, productID, inp)
}