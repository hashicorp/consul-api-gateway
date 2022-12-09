// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	ManagedLabel   = "api-gateway.consul.hashicorp.com/managed"
	nameLabel      = "api-gateway.consul.hashicorp.com/name"
	namespaceLabel = "api-gateway.consul.hashicorp.com/namespace"
	createdAtLabel = "api-gateway.consul.hashicorp.com/created"
)

func LabelsForGateway(gw *gwv1beta1.Gateway) map[string]string {
	return map[string]string{
		nameLabel:      gw.Name,
		namespaceLabel: gw.Namespace,
		createdAtLabel: fmt.Sprintf("%d", gw.CreationTimestamp.Unix()),
		ManagedLabel:   "true",
	}
}

func GatewayByLabels(object client.Object) types.NamespacedName {
	labels := object.GetLabels()
	return types.NamespacedName{
		Name:      labels[nameLabel],
		Namespace: labels[namespaceLabel],
	}
}

func IsManagedGateway(labels map[string]string) (string, bool) {
	managed, ok := labels[ManagedLabel]

	if !ok || managed != "true" {
		return "", false
	}
	name, ok := labels[nameLabel]
	if !ok {
		return "", false
	}
	return name, true
}
