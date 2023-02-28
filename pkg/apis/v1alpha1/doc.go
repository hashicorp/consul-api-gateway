// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +groupName=api-gateway.consul.hashicorp.com

const (
	Group   = "api-gateway.consul.hashicorp.com"
	Version = "v1alpha1"
)

var (
	GroupVersion = schema.GroupVersion{Group: Group, Version: Version}
)

func RegisterTypes(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(GroupVersion, &GatewayClassConfig{}, &GatewayClassConfigList{})
	scheme.AddKnownTypes(GroupVersion, &MeshService{}, &MeshServiceList{})
	meta.AddToGroupVersion(scheme, GroupVersion)
}
