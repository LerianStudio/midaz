package mocks

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
)

// MockSegmentsService is a mock of SegmentsService interface.
type MockSegmentsService struct {
	ctrl     *gomock.Controller
	recorder *MockSegmentsServiceMockRecorder
}

// MockSegmentsServiceMockRecorder is the mock recorder for MockSegmentsService.
type MockSegmentsServiceMockRecorder struct {
	mock *MockSegmentsService
}

// NewMockSegmentsService creates a new mock instance.
func NewMockSegmentsService(ctrl *gomock.Controller) *MockSegmentsService {
	mock := &MockSegmentsService{ctrl: ctrl}
	mock.recorder = &MockSegmentsServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSegmentsService) EXPECT() *MockSegmentsServiceMockRecorder {
	return m.recorder
}

// ListSegments mocks base method.
func (m *MockSegmentsService) ListSegments(ctx context.Context, organizationID, ledgerID, portfolioID string, opts *models.ListOptions) (*models.ListResponse[models.Segment], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListSegments", ctx, organizationID, ledgerID, portfolioID, opts)
	ret0, _ := ret[0].(*models.ListResponse[models.Segment])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListSegments indicates an expected call of ListSegments.
func (mr *MockSegmentsServiceMockRecorder) ListSegments(ctx, organizationID, ledgerID, portfolioID, opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListSegments", reflect.TypeOf((*MockSegmentsService)(nil).ListSegments), ctx, organizationID, ledgerID, portfolioID, opts)
}

// GetSegment mocks base method.
func (m *MockSegmentsService) GetSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSegment", ctx, organizationID, ledgerID, portfolioID, id)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSegment indicates an expected call of GetSegment.
func (mr *MockSegmentsServiceMockRecorder) GetSegment(ctx, organizationID, ledgerID, portfolioID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSegment", reflect.TypeOf((*MockSegmentsService)(nil).GetSegment), ctx, organizationID, ledgerID, portfolioID, id)
}

// CreateSegment mocks base method.
func (m *MockSegmentsService) CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateSegment", ctx, organizationID, ledgerID, portfolioID, input)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateSegment indicates an expected call of CreateSegment.
func (mr *MockSegmentsServiceMockRecorder) CreateSegment(ctx, organizationID, ledgerID, portfolioID, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateSegment", reflect.TypeOf((*MockSegmentsService)(nil).CreateSegment), ctx, organizationID, ledgerID, portfolioID, input)
}

// UpdateSegment mocks base method.
func (m *MockSegmentsService) UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string, input *models.UpdateSegmentInput) (*models.Segment, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateSegment", ctx, organizationID, ledgerID, portfolioID, id, input)
	ret0, _ := ret[0].(*models.Segment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateSegment indicates an expected call of UpdateSegment.
func (mr *MockSegmentsServiceMockRecorder) UpdateSegment(ctx, organizationID, ledgerID, portfolioID, id, input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateSegment", reflect.TypeOf((*MockSegmentsService)(nil).UpdateSegment), ctx, organizationID, ledgerID, portfolioID, id, input)
}

// DeleteSegment mocks base method.
func (m *MockSegmentsService) DeleteSegment(ctx context.Context, organizationID, ledgerID, portfolioID, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteSegment", ctx, organizationID, ledgerID, portfolioID, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteSegment indicates an expected call of DeleteSegment.
func (mr *MockSegmentsServiceMockRecorder) DeleteSegment(ctx, organizationID, ledgerID, portfolioID, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteSegment", reflect.TypeOf((*MockSegmentsService)(nil).DeleteSegment), ctx, organizationID, ledgerID, portfolioID, id)
}
