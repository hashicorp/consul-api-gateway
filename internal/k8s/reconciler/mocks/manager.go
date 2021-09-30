// Code generated by MockGen. DO NOT EDIT.
// Source: ./manager.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	reconciler "github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler"
	types "k8s.io/apimachinery/pkg/types"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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

// DeleteRoute mocks base method.
func (m *MockReconcileManager) DeleteRoute(ctx context.Context, name types.NamespacedName) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteRoute", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteRoute indicates an expected call of DeleteRoute.
func (mr *MockReconcileManagerMockRecorder) DeleteRoute(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteRoute", reflect.TypeOf((*MockReconcileManager)(nil).DeleteRoute), ctx, name)
}

// UpsertGateway mocks base method.
func (m *MockReconcileManager) UpsertGateway(ctx context.Context, g *v1alpha2.Gateway) error {
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
func (m *MockReconcileManager) UpsertGatewayClass(ctx context.Context, gc *v1alpha2.GatewayClass, validParameters bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertGatewayClass", ctx, gc, validParameters)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertGatewayClass indicates an expected call of UpsertGatewayClass.
func (mr *MockReconcileManagerMockRecorder) UpsertGatewayClass(ctx, gc, validParameters interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertGatewayClass", reflect.TypeOf((*MockReconcileManager)(nil).UpsertGatewayClass), ctx, gc, validParameters)
}

// UpsertRoute mocks base method.
func (m *MockReconcileManager) UpsertRoute(ctx context.Context, r reconciler.Route) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertRoute", ctx, r)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertRoute indicates an expected call of UpsertRoute.
func (mr *MockReconcileManagerMockRecorder) UpsertRoute(ctx, r interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertRoute", reflect.TypeOf((*MockReconcileManager)(nil).UpsertRoute), ctx, r)
}
