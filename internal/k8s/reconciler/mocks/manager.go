// Code generated by MockGen. DO NOT EDIT.
// Source: ./manager.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	types "k8s.io/apimachinery/pkg/types"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// MockReconcileManager is a mock of ReconcileManager interface.
type MockReconcileManager struct {
	ctrl     *gomock.Controller
	recorder *MockReconcileManagerMockRecorder
}

// MockReconcileManagerMockRecorder is the mock recorder for MockReconcileManager.
type MockReconcileManagerMockRecorder struct {
	mock *MockReconcileManager
}

// NewMockReconcileManager creates a new mock instance.
func NewMockReconcileManager(ctrl *gomock.Controller) *MockReconcileManager {
	mock := &MockReconcileManager{ctrl: ctrl}
	mock.recorder = &MockReconcileManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockReconcileManager) EXPECT() *MockReconcileManagerMockRecorder {
	return m.recorder
}

// DeleteGateway mocks base method.
func (m *MockReconcileManager) DeleteGateway(ctx context.Context, name types.NamespacedName) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteGateway", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteGateway indicates an expected call of DeleteGateway.
func (mr *MockReconcileManagerMockRecorder) DeleteGateway(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteGateway", reflect.TypeOf((*MockReconcileManager)(nil).DeleteGateway), ctx, name)
}

// DeleteGatewayClass mocks base method.
func (m *MockReconcileManager) DeleteGatewayClass(ctx context.Context, name string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteGatewayClass", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteGatewayClass indicates an expected call of DeleteGatewayClass.
func (mr *MockReconcileManagerMockRecorder) DeleteGatewayClass(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteGatewayClass", reflect.TypeOf((*MockReconcileManager)(nil).DeleteGatewayClass), ctx, name)
}

// DeleteHTTPRoute mocks base method.
func (m *MockReconcileManager) DeleteHTTPRoute(ctx context.Context, name types.NamespacedName) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteHTTPRoute", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteHTTPRoute indicates an expected call of DeleteHTTPRoute.
func (mr *MockReconcileManagerMockRecorder) DeleteHTTPRoute(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteHTTPRoute", reflect.TypeOf((*MockReconcileManager)(nil).DeleteHTTPRoute), ctx, name)
}

// DeleteTCPRoute mocks base method.
func (m *MockReconcileManager) DeleteTCPRoute(ctx context.Context, name types.NamespacedName) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteTCPRoute", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteTCPRoute indicates an expected call of DeleteTCPRoute.
func (mr *MockReconcileManagerMockRecorder) DeleteTCPRoute(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteTCPRoute", reflect.TypeOf((*MockReconcileManager)(nil).DeleteTCPRoute), ctx, name)
}

// UpsertGateway mocks base method.
func (m *MockReconcileManager) UpsertGateway(ctx context.Context, g *v1beta1.Gateway) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertGateway", ctx, g)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertGateway indicates an expected call of UpsertGateway.
func (mr *MockReconcileManagerMockRecorder) UpsertGateway(ctx, g interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertGateway", reflect.TypeOf((*MockReconcileManager)(nil).UpsertGateway), ctx, g)
}

// UpsertGatewayClass mocks base method.
func (m *MockReconcileManager) UpsertGatewayClass(ctx context.Context, gc *v1beta1.GatewayClass) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertGatewayClass", ctx, gc)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertGatewayClass indicates an expected call of UpsertGatewayClass.
func (mr *MockReconcileManagerMockRecorder) UpsertGatewayClass(ctx, gc interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertGatewayClass", reflect.TypeOf((*MockReconcileManager)(nil).UpsertGatewayClass), ctx, gc)
}

// UpsertHTTPRoute mocks base method.
func (m *MockReconcileManager) UpsertHTTPRoute(ctx context.Context, r *v1alpha2.HTTPRoute) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertHTTPRoute", ctx, r)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertHTTPRoute indicates an expected call of UpsertHTTPRoute.
func (mr *MockReconcileManagerMockRecorder) UpsertHTTPRoute(ctx, r interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertHTTPRoute", reflect.TypeOf((*MockReconcileManager)(nil).UpsertHTTPRoute), ctx, r)
}

// UpsertTCPRoute mocks base method.
func (m *MockReconcileManager) UpsertTCPRoute(ctx context.Context, r *v1alpha2.TCPRoute) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertTCPRoute", ctx, r)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertTCPRoute indicates an expected call of UpsertTCPRoute.
func (mr *MockReconcileManagerMockRecorder) UpsertTCPRoute(ctx, r interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertTCPRoute", reflect.TypeOf((*MockReconcileManager)(nil).UpsertTCPRoute), ctx, r)
}
