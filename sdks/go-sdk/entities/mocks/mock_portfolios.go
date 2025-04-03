package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockPortfoliosService is a mock of PortfoliosService interface.
type MockPortfoliosService struct {
	ctrl     *gomock.Controller
	recorder *MockPortfoliosServiceMockRecorder
}

// MockPortfoliosServiceMockRecorder is the mock recorder for MockPortfoliosService.
type MockPortfoliosServiceMockRecorder struct {
	mock *MockPortfoliosService
}

// NewMockPortfoliosService creates a new mock instance.
func NewMockPortfoliosService(ctrl *gomock.Controller) *MockPortfoliosService {
	mock := &MockPortfoliosService{ctrl: ctrl}

	mock.recorder = &MockPortfoliosServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPortfoliosService) EXPECT() *MockPortfoliosServiceMockRecorder {
	return m.recorder
}

// ListPortfolios mocks base method.
func (m *MockPortfoliosService) ListPortfolios(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Portfolio], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListPortfolios", ctx, organizationID, ledgerID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Portfolio])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListPortfolios indicates an expected call of ListPortfolios.
func (mr *MockPortfoliosServiceMockRecorder) ListPortfolios(ctx, organizationID, ledgerID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPortfolios", reflect.TypeOf((*MockPortfoliosService)(nil).ListPortfolios), ctx, organizationID, ledgerID, opts)
}

// GetPortfolio mocks base method.
func (m *MockPortfoliosService) GetPortfolio(ctx context.Context, organizationID, ledgerID, id string) (*models.Portfolio, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPortfolio", ctx, organizationID, ledgerID, id)
	ret0, _ := ret[0].(*models.Portfolio)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPortfolio indicates an expected call of GetPortfolio.
func (mr *MockPortfoliosServiceMockRecorder) GetPortfolio(ctx, organizationID, ledgerID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPortfolio", reflect.TypeOf((*MockPortfoliosService)(nil).GetPortfolio), ctx, organizationID, ledgerID, id)
}

// CreatePortfolio mocks base method.
func (m *MockPortfoliosService) CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePortfolio", ctx, organizationID, ledgerID, input)
	ret0, _ := ret[0].(*models.Portfolio)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePortfolio indicates an expected call of CreatePortfolio.
func (mr *MockPortfoliosServiceMockRecorder) CreatePortfolio(ctx, organizationID, ledgerID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePortfolio", reflect.TypeOf((*MockPortfoliosService)(nil).CreatePortfolio), ctx, organizationID, ledgerID, input)
}

// UpdatePortfolio mocks base method.
func (m *MockPortfoliosService) UpdatePortfolio(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdatePortfolio", ctx, organizationID, ledgerID, id, input)
	ret0, _ := ret[0].(*models.Portfolio)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdatePortfolio indicates an expected call of UpdatePortfolio.
func (mr *MockPortfoliosServiceMockRecorder) UpdatePortfolio(ctx, organizationID, ledgerID, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdatePortfolio", reflect.TypeOf((*MockPortfoliosService)(nil).UpdatePortfolio), ctx, organizationID, ledgerID, id, input)
}

// DeletePortfolio mocks base method.
func (m *MockPortfoliosService) DeletePortfolio(ctx context.Context, organizationID, ledgerID, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeletePortfolio", ctx, organizationID, ledgerID, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeletePortfolio indicates an expected call of DeletePortfolio.
func (mr *MockPortfoliosServiceMockRecorder) DeletePortfolio(ctx, organizationID, ledgerID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeletePortfolio", reflect.TypeOf((*MockPortfoliosService)(nil).DeletePortfolio), ctx, organizationID, ledgerID, id)
}

// ListSegments mocks base method.
func (m *MockPortfoliosService) ListSegments(ctx context.Context, organizationID, ledgerID, portfolioID string, opts *models.ListOptions) (*models.ListResponse[models.Segment], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListSegments", ctx, organizationID, ledgerID, portfolioID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Segment])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListSegments indicates an expected call of ListSegments.
func (mr *MockPortfoliosServiceMockRecorder) ListSegments(ctx, organizationID, ledgerID, portfolioID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListSegments", reflect.TypeOf((*MockPortfoliosService)(nil).ListSegments), ctx, organizationID, ledgerID, portfolioID, opts)
}

// GetSegment mocks base method.
func (m *MockPortfoliosService) GetSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSegment", ctx, organizationID, ledgerID, portfolioID, id)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSegment indicates an expected call of GetSegment.
func (mr *MockPortfoliosServiceMockRecorder) GetSegment(ctx, organizationID, ledgerID, portfolioID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSegment", reflect.TypeOf((*MockPortfoliosService)(nil).GetSegment), ctx, organizationID, ledgerID, portfolioID, id)
}

// CreateSegment mocks base method.
func (m *MockPortfoliosService) CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateSegment", ctx, organizationID, ledgerID, portfolioID, input)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateSegment indicates an expected call of CreateSegment.
func (mr *MockPortfoliosServiceMockRecorder) CreateSegment(ctx, organizationID, ledgerID, portfolioID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateSegment", reflect.TypeOf((*MockPortfoliosService)(nil).CreateSegment), ctx, organizationID, ledgerID, portfolioID, input)
}

// UpdateSegment mocks base method.
func (m *MockPortfoliosService) UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string, input *models.UpdateSegmentInput) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateSegment", ctx, organizationID, ledgerID, portfolioID, id, input)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateSegment indicates an expected call of UpdateSegment.
func (mr *MockPortfoliosServiceMockRecorder) UpdateSegment(ctx, organizationID, ledgerID, portfolioID, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateSegment", reflect.TypeOf((*MockPortfoliosService)(nil).UpdateSegment), ctx, organizationID, ledgerID, portfolioID, id, input)
}

// DeleteSegment mocks base method.
func (m *MockPortfoliosService) DeleteSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteSegment", ctx, organizationID, ledgerID, portfolioID, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteSegment indicates an expected call of DeleteSegment.
func (mr *MockPortfoliosServiceMockRecorder) DeleteSegment(ctx, organizationID, ledgerID, portfolioID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteSegment", reflect.TypeOf((*MockPortfoliosService)(nil).DeleteSegment), ctx, organizationID, ledgerID, portfolioID, id)
}
