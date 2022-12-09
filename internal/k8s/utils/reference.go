// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	gatewayGroup = gwv1alpha2.Group(gwv1beta1.GroupName)
	gatewayKind  = gwv1alpha2.Kind("Gateway")
)

func ReferencesGateway(namespace string, ref gwv1alpha2.ParentReference) (types.NamespacedName, bool) {
	if ref.Group != nil && *ref.Group != gatewayGroup {
		return types.NamespacedName{}, false
	}
	if ref.Kind != nil && *ref.Kind != gatewayKind {
		return types.NamespacedName{}, false
	}

	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return types.NamespacedName{Name: string(ref.Name), Namespace: namespace}, true
}
