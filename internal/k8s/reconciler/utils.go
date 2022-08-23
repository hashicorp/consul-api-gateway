package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel = "kubernetes.io/metadata.name"
)

func routeMatchesListener(listenerName gwv1beta1.SectionName, routeSectionName *gwv1alpha2.SectionName) (can bool, must bool) {
	if routeSectionName == nil {
		return true, false
	}
	return string(listenerName) == string(*routeSectionName), true
}

func routeMatchesListenerHostname(listenerHostname *gwv1beta1.Hostname, hostnames []gwv1alpha2.Hostname) bool {
	if listenerHostname == nil || len(hostnames) == 0 {
		return true
	}

	for _, name := range hostnames {
		if hostnamesMatch(name, *listenerHostname) {
			return true
		}
	}
	return false
}

func hostnamesMatch(a gwv1alpha2.Hostname, b gwv1beta1.Hostname) bool {
	if a == "" || a == "*" || b == "" || b == "*" {
		// any wildcard always matches
		return true
	}

	if strings.HasPrefix(string(a), "*.") || strings.HasPrefix(string(b), "*.") {
		aLabels, bLabels := strings.Split(string(a), "."), strings.Split(string(b), ".")
		if len(aLabels) != len(bLabels) {
			return false
		}

		for i := 1; i < len(aLabels); i++ {
			if !strings.EqualFold(aLabels[i], bLabels[i]) {
				return false
			}
		}
		return true
	}

	return string(a) == string(b)
}

func routeKindIsAllowedForListener(kinds []gwv1beta1.RouteGroupKind, route *K8sRoute) bool {
	if kinds == nil {
		return true
	}

	gvk := route.GroupVersionKind()
	for _, kind := range kinds {
		group := gwv1beta1.GroupName
		if kind.Group != nil && *kind.Group != "" {
			group = string(*kind.Group)
		}
		if string(kind.Kind) == gvk.Kind && group == gvk.Group {
			return true
		}
	}

	return false
}

// routeAllowedForListenerNamespaces determines whether the route is allowed
// to bind to the Gateway based on the AllowedRoutes namespace selectors.
func routeAllowedForListenerNamespaces(ctx context.Context, gatewayNS string, allowedRoutes *gwv1beta1.AllowedRoutes, route *K8sRoute, c gatewayclient.Client) (bool, error) {
	var namespaceSelector *gwv1beta1.RouteNamespaces
	if allowedRoutes != nil {
		// check gateway namespace
		namespaceSelector = allowedRoutes.Namespaces
	}

	// set default if namespace selector is nil
	from := gwv1beta1.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gwv1beta1.NamespacesFromAll:
		return true, nil
	case gwv1beta1.NamespacesFromSame:
		return gatewayNS == route.GetNamespace(), nil
	case gwv1beta1.NamespacesFromSelector:
		namespaceSelector, err := metav1.LabelSelectorAsSelector(namespaceSelector.Selector)
		if err != nil {
			return false, fmt.Errorf("error parsing label selector: %w", err)
		}

		// retrieve the route's namespace and determine whether selector matches
		namespace, err := c.GetNamespace(ctx, types.NamespacedName{Name: route.GetNamespace()})
		if err != nil {
			return false, fmt.Errorf("error retrieving namespace for route: %w", err)
		}

		return namespaceSelector.Matches(toNamespaceSet(namespace.GetName(), namespace.GetLabels())), nil
	}
	return false, nil
}

// routeAllowedForBackendRef determines whether the route is allowed
// for the backend either by being in the same namespace or by having
// an applicable ReferenceGrant in the same namespace as the backend.
//
// TODO This func is currently called once for each backendRef on a route and results
//   in fetching ReferenceGrants more than we technically have to in some cases
func routeAllowedForBackendRef(ctx context.Context, route Route, backendRef gwv1alpha2.BackendRef, c gatewayclient.Client) (bool, error) {
	fromNS := route.GetNamespace()
	fromGK := metav1.GroupKind{
		Group: route.GroupVersionKind().Group,
		Kind:  route.GroupVersionKind().Kind,
	}

	toName := string(backendRef.Name)
	toNS := ""
	if backendRef.Namespace != nil {
		toNS = string(*backendRef.Namespace)
	}

	// Kind should default to Service if not set
	// https://github.com/kubernetes-sigs/gateway-api/blob/ef773194892636ea8ecbb2b294daf771d4dd5009/apis/v1alpha2/object_reference_types.go#L105
	toGK := metav1.GroupKind{Kind: "Service"}
	if backendRef.Group != nil {
		toGK.Group = string(*backendRef.Group)
	}
	if backendRef.Kind != nil {
		toGK.Kind = string(*backendRef.Kind)
	}

	return referenceAllowed(ctx, fromGK, fromNS, toGK, toNS, toName, c)
}

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

func toNamespaceSet(name string, labels map[string]string) klabels.Labels {
	// If namespace label is not set, implicitly insert it to support older Kubernetes versions
	if labels[NamespaceNameLabel] == name {
		// Already set, avoid copies
		return klabels.Set(labels)
	}
	// First we need a copy to not modify the underlying object
	ret := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		ret[k] = v
	}
	ret[NamespaceNameLabel] = name
	return klabels.Set(ret)
}

func sortParents(parents []gwv1alpha2.RouteParentStatus) []gwv1alpha2.RouteParentStatus {
	for _, parent := range parents {
		sort.SliceStable(parent.Conditions, func(i, j int) bool {
			return asJSON(parent.Conditions[i]) < asJSON(parent.Conditions[j])
		})
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return asJSON(parents[i]) < asJSON(parents[j])
	})
	return parents
}

func asJSON(item interface{}) string {
	data, err := json.Marshal(item)
	if err != nil {
		// everything passed to this internally should be
		// serializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return string(data)
}

func parseParent(stringified string) gwv1alpha2.ParentReference {
	var ref gwv1alpha2.ParentReference
	if err := json.Unmarshal([]byte(stringified), &ref); err != nil {
		// everything passed to this internally should be
		// deserializable, if something is passed to it that
		// isn't, just panic since it's a usage error at
		// that point
		panic(err)
	}
	return ref
}

func conditionEqual(a, b metav1.Condition) bool {
	if a.Type != b.Type ||
		a.ObservedGeneration != b.ObservedGeneration ||
		a.Status != b.Status ||
		a.Reason != b.Reason ||
		a.Message != b.Message {
		return false
	}
	return true
}

func conditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}

	for i, newCondition := range a {
		if !conditionEqual(newCondition, b[i]) {
			return false
		}
	}
	return true
}

func listenerStatusEqual(a, b gwv1beta1.ListenerStatus) bool {
	if a.Name != b.Name {
		return false
	}
	if !reflect.DeepEqual(a.SupportedKinds, b.SupportedKinds) {
		return false
	}
	if a.AttachedRoutes != b.AttachedRoutes {
		return false
	}
	return conditionsEqual(a.Conditions, b.Conditions)
}

func listenerStatusesEqual(a, b []gwv1beta1.ListenerStatus) bool {
	if len(a) != len(b) {
		// we have a different number of conditions, so they aren't the same
		return false
	}
	for i, newStatus := range a {
		if !listenerStatusEqual(newStatus, b[i]) {
			return false
		}
	}
	return true
}

func parentStatusEqual(a, b gwv1alpha2.RouteParentStatus) bool {
	if a.ControllerName != b.ControllerName {
		return false
	}
	if asJSON(a.ParentRef) != asJSON(b.ParentRef) {
		return false
	}

	return conditionsEqual(a.Conditions, b.Conditions)
}

func routeStatusEqual(a, b gwv1alpha2.RouteStatus) bool {
	if len(a.Parents) != len(b.Parents) {
		return false
	}

	for i, oldParent := range a.Parents {
		if !parentStatusEqual(oldParent, b.Parents[i]) {
			return false
		}
	}
	return true
}

func gatewayStatusEqual(a, b gwv1beta1.GatewayStatus) bool {
	if !conditionsEqual(a.Conditions, b.Conditions) {
		return false
	}

	if !listenerStatusesEqual(a.Listeners, b.Listeners) {
		return false
	}

	return true
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
