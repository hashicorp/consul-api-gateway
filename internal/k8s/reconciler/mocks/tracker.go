// Code generated by MockGen. DO NOT EDIT.
// Source: ./tracker.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1 "k8s.io/api/core/v1"
	v10 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

// MockGatewayStatusTracker is a mock of GatewayStatusTracker interface.
type MockGatewayStatusTracker struct {
	ctrl     *gomock.Controller
	recorder *MockGatewayStatusTrackerMockRecorder
}

// MockGatewayStatusTrackerMockRecorder is the mock recorder for MockGatewayStatusTracker.
type MockGatewayStatusTrackerMockRecorder struct {
	mock *MockGatewayStatusTracker
}

// NewMockGatewayStatusTracker creates a new mock instance.
func NewMockGatewayStatusTracker(ctrl *gomock.Controller) *MockGatewayStatusTracker {
	mock := &MockGatewayStatusTracker{ctrl: ctrl}
	mock.recorder = &MockGatewayStatusTrackerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGatewayStatusTracker) EXPECT() *MockGatewayStatusTrackerMockRecorder {
	return m.recorder
}

// DeleteStatus mocks base method.
func (m *MockGatewayStatusTracker) DeleteStatus(name types.NamespacedName) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "DeleteStatus", name)
}

// DeleteStatus indicates an expected call of DeleteStatus.
func (mr *MockGatewayStatusTrackerMockRecorder) DeleteStatus(name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteStatus", reflect.TypeOf((*MockGatewayStatusTracker)(nil).DeleteStatus), name)
}

// UpdateStatus mocks base method.
func (m *MockGatewayStatusTracker) UpdateStatus(name types.NamespacedName, pod *v1.Pod, conditions []v10.Condition, force bool, cb func() error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateStatus", name, pod, conditions, force, cb)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateStatus indicates an expected call of UpdateStatus.
func (mr *MockGatewayStatusTrackerMockRecorder) UpdateStatus(name, pod, conditions, force, cb interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateStatus", reflect.TypeOf((*MockGatewayStatusTracker)(nil).UpdateStatus), name, pod, conditions, force, cb)
}
