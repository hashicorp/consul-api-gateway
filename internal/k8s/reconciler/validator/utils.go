// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validator

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
)

// referenceAllowed checks to see if a reference between resources is allowed.
// In particular, references from one namespace to a resource in a different namespace
// require an applicable ReferenceGrant be found in the namespace containing the resource
// being referred to.
//
// For example, a Gateway in namespace "foo" may only reference a Secret in namespace "bar"
// if a ReferenceGrant in namespace "bar" allows references from namespace "foo".
func referenceAllowed(ctx context.Context, fromGK metav1.GroupKind, fromNamespace string, toGK metav1.GroupKind, toNamespace, toName string, c gatewayclient.Client) (bool, error) {
	// Reference does not cross namespaces
	if toNamespace == "" || toNamespace == fromNamespace {
		return true, nil
	}

	// Fetch all ReferenceGrants in the referenced namespace
	refGrants, err := c.GetReferenceGrantsInNamespace(ctx, toNamespace)
	if err != nil || len(refGrants) == 0 {
		return false, err
	}

	for _, refGrant := range refGrants {
		// Check for a From that applies
		fromMatch := false
		for _, from := range refGrant.Spec.From {
			if fromGK.Group == string(from.Group) && fromGK.Kind == string(from.Kind) && fromNamespace == string(from.Namespace) {
				fromMatch = true
				break
			}
		}

		if !fromMatch {
			continue
		}

		// Check for a To that applies
		for _, to := range refGrant.Spec.To {
			if toGK.Group == string(to.Group) && toGK.Kind == string(to.Kind) {
				if to.Name == nil || *to.Name == "" {
					// No name specified is treated as a wildcard within the namespace
					return true, nil
				}

				if gwv1alpha2.ObjectName(toName) == *to.Name {
					// The ReferenceGrant specifically targets this object
					return true, nil
				}
			}
		}
	}

	// No ReferenceGrant was found which allows this cross-namespace reference
	return false, nil
}

type gwObjectName interface {
	gwv1beta1.ObjectName | gwv1alpha2.ObjectName
}

type gwNamespace interface {
	gwv1beta1.Namespace | gwv1alpha2.Namespace
}

// getNamespacedName returns types.NamespacedName defaulted to a parent
// namespace in the case where the provided namespace is nil.
func getNamespacedName[O gwObjectName, N gwNamespace](name O, namespace *N, parentNamespace string) types.NamespacedName {
	if namespace != nil {
		return types.NamespacedName{Namespace: string(*namespace), Name: string(name)}
	}
	return types.NamespacedName{Namespace: parentNamespace, Name: string(name)}
}
