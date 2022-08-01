// Code generated by MockGen. DO NOT EDIT.
// Source: ./certificates.go

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	api "github.com/hashicorp/vault/api"
)

// MockLogicalClient is a mock of LogicalClient interface.
type MockLogicalClient struct {
	ctrl     *gomock.Controller
	recorder *MockLogicalClientMockRecorder
}

// MockLogicalClientMockRecorder is the mock recorder for MockLogicalClient.
type MockLogicalClientMockRecorder struct {
	mock *MockLogicalClient
}

// NewMockLogicalClient creates a new mock instance.
func NewMockLogicalClient(ctrl *gomock.Controller) *MockLogicalClient {
	mock := &MockLogicalClient{ctrl: ctrl}
	mock.recorder = &MockLogicalClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLogicalClient) EXPECT() *MockLogicalClientMockRecorder {
	return m.recorder
}

// WriteWithContext mocks base method.
func (m *MockLogicalClient) WriteWithContext(arg0 context.Context, arg1 string, arg2 map[string]interface{}) (*api.Secret, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteWithContext", arg0, arg1, arg2)
	ret0, _ := ret[0].(*api.Secret)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// WriteWithContext indicates an expected call of WriteWithContext.
func (mr *MockLogicalClientMockRecorder) WriteWithContext(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WriteWithContext", reflect.TypeOf((*MockLogicalClient)(nil).WriteWithContext), arg0, arg1, arg2)
}
