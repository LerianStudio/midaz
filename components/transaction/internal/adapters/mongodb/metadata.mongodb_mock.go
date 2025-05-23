// Code generated by MockGen. DO NOT EDIT.
// Source: ./components/transaction/internal/adapters/mongodb/metadata.mongodb.go
//
// Generated by this command:
//
//	mockgen -source=./components/transaction/internal/adapters/mongodb/metadata.mongodb.go -destination=./components/transaction/internal/adapters/mongodb/metadata.mongodb_mock.go -package=mongodb
//

// Package mongodb is a generated GoMock package.
package mongodb

import (
	context "context"
	reflect "reflect"

	http "github.com/LerianStudio/midaz/pkg/net/http"
	gomock "go.uber.org/mock/gomock"
)

// MockRepository is a mock of Repository interface.
type MockRepository struct {
	ctrl     *gomock.Controller
	recorder *MockRepositoryMockRecorder
	isgomock struct{}
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

// Create mocks base method.
func (m *MockRepository) Create(ctx context.Context, collection string, metadata *Metadata) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, collection, metadata)
	ret0, _ := ret[0].(error)
	return ret0
}

// Create indicates an expected call of Create.
func (mr *MockRepositoryMockRecorder) Create(ctx, collection, metadata any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockRepository)(nil).Create), ctx, collection, metadata)
}

// Delete mocks base method.
func (m *MockRepository) Delete(ctx context.Context, collection, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, collection, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockRepositoryMockRecorder) Delete(ctx, collection, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockRepository)(nil).Delete), ctx, collection, id)
}

// FindByEntity mocks base method.
func (m *MockRepository) FindByEntity(ctx context.Context, collection, id string) (*Metadata, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindByEntity", ctx, collection, id)
	ret0, _ := ret[0].(*Metadata)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindByEntity indicates an expected call of FindByEntity.
func (mr *MockRepositoryMockRecorder) FindByEntity(ctx, collection, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindByEntity", reflect.TypeOf((*MockRepository)(nil).FindByEntity), ctx, collection, id)
}

// FindList mocks base method.
func (m *MockRepository) FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindList", ctx, collection, filter)
	ret0, _ := ret[0].([]*Metadata)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindList indicates an expected call of FindList.
func (mr *MockRepositoryMockRecorder) FindList(ctx, collection, filter any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindList", reflect.TypeOf((*MockRepository)(nil).FindList), ctx, collection, filter)
}

// Update mocks base method.
func (m *MockRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Update", ctx, collection, id, metadata)
	ret0, _ := ret[0].(error)
	return ret0
}

// Update indicates an expected call of Update.
func (mr *MockRepositoryMockRecorder) Update(ctx, collection, id, metadata any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Update", reflect.TypeOf((*MockRepository)(nil).Update), ctx, collection, id, metadata)
}
