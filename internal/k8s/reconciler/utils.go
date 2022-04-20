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

	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient"
)

const (
	// NamespaceNameLabel represents that label added automatically to namespaces is newer Kubernetes clusters
	NamespaceNameLabel = "kubernetes.io/metadata.name"
)

func routeMatchesListener(listenerName gw.SectionName, sectionName *gw.SectionName) (bool, bool) {
	if sectionName == nil {
		return true, false
	}
	return listenerName == *sectionName, true
}

func routeMatchesListenerHostname(listenerHostname *gw.Hostname, hostnames []gw.Hostname) bool {
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

func hostnamesMatch(a, b gw.Hostname) bool {
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

	return a == b
}

func routeKindIsAllowedForListener(kinds []gw.RouteGroupKind, route *K8sRoute) bool {
	if kinds == nil {
		return true
	}

	gvk := route.GroupVersionKind()
	for _, kind := range kinds {
		group := gw.GroupName
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
func routeAllowedForListenerNamespaces(ctx context.Context, gatewayNS string, allowedRoutes *gw.AllowedRoutes, route *K8sRoute, c gatewayclient.Client) (bool, error) {
	var namespaceSelector *gw.RouteNamespaces
	if allowedRoutes != nil {
		// check gateway namespace
		namespaceSelector = allowedRoutes.Namespaces
	}

	// set default if namespace selector is nil
	from := gw.NamespacesFromSame
	if namespaceSelector != nil && namespaceSelector.From != nil && *namespaceSelector.From != "" {
		from = *namespaceSelector.From
	}
	switch from {
	case gw.NamespacesFromAll:
		return true, nil
	case gw.NamespacesFromSame:
		return gatewayNS == route.GetNamespace(), nil
	case gw.NamespacesFromSelector:
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

// gatewayAllowedForSecretRef determines whether the gateway is allowed
// for the secret either by being in the same namespace or by having
// an applicable ReferencePolicy in the same namespace as the secret.
func gatewayAllowedForSecretRef(ctx context.Context, gateway *gw.Gateway, secretRef gw.SecretObjectReference, c gatewayclient.Client) (bool, error) {
	secretNS := ""
	if secretRef.Namespace != nil {
		secretNS = string(*secretRef.Namespace)
	}

	// Allow if gateway and secret are in the same namespace
	if secretNS == "" || gateway.GetNamespace() == secretNS {
		return true, nil
	}

	// Allow if ReferencePolicy present for gateway + secret combination
	refPolicies, err := c.GetReferencePoliciesInNamespace(ctx, secretNS)
	if err != nil || len(refPolicies) == 0 {
		return false, err
	}

	for _, refPolicy := range refPolicies {
		// Check for a From that applies to the route
		validFrom := false
		for _, from := range refPolicy.Spec.From {
			// If this policy allows the group, kind and namespace for this gateway
			if gateway.GroupVersionKind().Group == string(from.Group) &&
				gateway.GroupVersionKind().Kind == string(from.Kind) &&
				gateway.GetNamespace() == string(from.Namespace) {
				validFrom = true
				break
			}
		}

		// If this ReferencePolicy has no applicable From, no need to check for a To
		if !validFrom {
			continue
		}

		var secretRefGroup gw.Group
		if secretRef.Group != nil {
			secretRefGroup = *secretRef.Group
		}

		// Backend kind should default to Secret if not set
		// https://github.com/kubernetes-sigs/gateway-api/blob/ef773194892636ea8ecbb2b294daf771d4dd5009/apis/v1alpha2/object_reference_types.go#L59
		var secretRefKind gw.Kind = "Secret"
		if secretRef.Kind != nil {
			secretRefKind = *secretRef.Kind
		}

		// Check for a To that applies to the secretRef
		for _, to := range refPolicy.Spec.To {
			// If this policy allows the group, kind, and name for this backend
			if to.Group == secretRefGroup &&
				to.Kind == secretRefKind &&
				(to.Name == nil || *to.Name == secretRef.Name) {
				return true, nil
			}
		}
	}

	return false, err
}

// routeAllowedForBackendRef determines whether the route is allowed
// for the backend either by being in the same namespace or by having
// an applicable ReferencePolicy in the same namespace as the backend.
//
// TODO This func is currently called once for each backendRef on a route and results
//   in fetching ReferencePolicies more than we technically have to in some cases
func routeAllowedForBackendRef(ctx context.Context, route Route, backendRef gw.BackendRef, c gatewayclient.Client) (bool, error) {
	backendNamespace := ""
	if backendRef.Namespace != nil {
		backendNamespace = string(*backendRef.Namespace)
	}

	// Allow if route and backend are in the same namespace
	if backendNamespace == "" || route.GetNamespace() == backendNamespace {
		return true, nil
	}

	// Allow if ReferencePolicy present for route + backend combination
	refPolicies, err := c.GetReferencePoliciesInNamespace(ctx, backendNamespace)
	if err != nil || len(refPolicies) == 0 {
		return false, err
	}

	for _, refPolicy := range refPolicies {
		// Check for a From that applies to the route
		validFrom := false
		for _, from := range refPolicy.Spec.From {
			// If this policy allows the group, kind and namespace for this route
			if route.GroupVersionKind().Group == string(from.Group) &&
				route.GroupVersionKind().Kind == string(from.Kind) &&
				route.GetNamespace() == string(from.Namespace) {
				validFrom = true
				break
			}
		}

		// If this ReferencePolicy has no applicable From, no need to check for a To
		if !validFrom {
			continue
		}

		var backendRefGroup gw.Group
		if backendRef.Group != nil {
			backendRefGroup = *backendRef.Group
		}

		// Backend kind should default to Service if not set
		// https://github.com/kubernetes-sigs/gateway-api/blob/ef773194892636ea8ecbb2b294daf771d4dd5009/apis/v1alpha2/object_reference_types.go#L105
		backendRefKind := gw.Kind("Service")
		if backendRef.Kind != nil {
			backendRefKind = *backendRef.Kind
		}

		// Check for a To that applies to the backendRef
		for _, to := range refPolicy.Spec.To {
			// If this policy allows the group, kind, and name for this backend
			if to.Group == backendRefGroup &&
				to.Kind == backendRefKind &&
				(to.Name == nil || *to.Name == backendRef.Name) {
				return true, nil
			}
		}
	}

	return false, err
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

func sortParents(parents []gw.RouteParentStatus) []gw.RouteParentStatus {
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

func parseParent(stringified string) gw.ParentRef {
	var ref gw.ParentRef
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

func listenerStatusEqual(a, b gw.ListenerStatus) bool {
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

func listenerStatusesEqual(a, b []gw.ListenerStatus) bool {
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

func parentStatusEqual(a, b gw.RouteParentStatus) bool {
	if a.ControllerName != b.ControllerName {
		return false
	}
	if asJSON(a.ParentRef) != asJSON(b.ParentRef) {
		return false
	}

	return conditionsEqual(a.Conditions, b.Conditions)
}

func routeStatusEqual(a, b gw.RouteStatus) bool {
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

func gatewayStatusEqual(a, b gw.GatewayStatus) bool {
	if !conditionsEqual(a.Conditions, b.Conditions) {
		return false
	}

	if !listenerStatusesEqual(a.Listeners, b.Listeners) {
		return false
	}

	return true
}

// getNamespacedName returns types.NamespacedName defaulted to a parent
// namespace in the case where the provided namespace is nil.
func getNamespacedName(name gw.ObjectName, namespace *gw.Namespace, parentNamespace string) types.NamespacedName {
	if namespace != nil {
		return types.NamespacedName{Namespace: string(*namespace), Name: string(name)}
	}
	return types.NamespacedName{Namespace: parentNamespace, Name: string(name)}
}
